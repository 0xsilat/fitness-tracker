package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func TestActivityPresentation(t *testing.T) {
	if got := activityLevel(0); got != "activity-cell level-0" {
		t.Fatalf("zero level=%q", got)
	}
	if got := activityLevel(3); got != "activity-cell level-3" {
		t.Fatalf("high level=%q", got)
	}
	day := domain.ActivityDay{Date: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC), Sessions: 1}
	if got := activityLabel(day); got != "24 Aug 2026: 1 session" {
		t.Fatalf("label=%q", got)
	}
	if got := activityOffset([]domain.ActivityDay{day}); got != 0 {
		t.Fatalf("Monday offset=%d", got)
	}
}

func TestActivityMonthMarkersAlignToMondayBasedWeeks(t *testing.T) {
	start := time.Date(2025, 8, 29, 0, 0, 0, 0, time.UTC) // Friday.
	days := make([]domain.ActivityDay, 365)
	for i := range days {
		days[i].Date = start.AddDate(0, 0, i)
	}

	if got := activityWeekCount(days); got != 53 {
		t.Fatalf("week count=%d want 53", got)
	}
	markers := activityMonthMarkers(days)
	if len(markers) != 12 {
		t.Fatalf("month markers=%d want 12", len(markers))
	}
	if got := markers[0]; got.Label != "Sep" || got.Column != 2 {
		t.Fatalf("first marker=%#v want Sep in column 2", got)
	}
	if got := markers[4]; got.Label != "Jan" || got.Column != 19 {
		t.Fatalf("year-boundary marker=%#v want Jan in column 19", got)
	}
}

func TestSessionMonthBoundariesIncludeYear(t *testing.T) {
	items := []domain.Session{
		{PerformedOn: time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC)},
		{PerformedOn: time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC)},
		{PerformedOn: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)},
		{PerformedOn: time.Date(2025, 7, 31, 0, 0, 0, 0, time.UTC)},
	}
	want := []bool{true, false, true, true}
	for i := range items {
		if got := startsSessionMonth(items, i); got != want[i] {
			t.Errorf("index %d boundary=%v want %v", i, got, want[i])
		}
	}
	if got := sessionMonth(items[0].PerformedOn); got != "August 2026" {
		t.Fatalf("month label=%q", got)
	}
}

func TestSessionRowContainsCompactSummary(t *testing.T) {
	session := domain.Session{ID: 42, WorkoutName: "Lower A", RoutineName: "Hidden routine", Format: "mixed", Status: "completed", CompletedSets: 5, PerformedOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}
	var output bytes.Buffer
	if err := SessionRow(session).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Lower A", "24 Aug 2026", "completed", "/sessions/42", "5 sets done"} {
		if !strings.Contains(html, want) {
			t.Errorf("row missing %q: %s", want, html)
		}
	}
	for _, unwanted := range []string{"Hidden routine", "Mixed formats"} {
		if strings.Contains(html, unwanted) {
			t.Errorf("row unexpectedly contains %q: %s", unwanted, html)
		}
	}
}

func TestCompletedSessionHasEditMode(t *testing.T) {
	session := domain.Session{ID: 42, WorkoutName: "Lower A", RoutineName: "Strength", Format: "mixed", Status: "completed", CompletedSets: 2, PerformedOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}
	exercises := []domain.SessionExercise{{ID: 7, SessionID: 42, Name: "Squat", Mode: "weighted", Format: "sets_reps", Sets: []domain.SessionSet{{ID: 11, Position: 1}, {ID: 12, Position: 2}}}}
	var view bytes.Buffer
	if err := SessionPage(session, exercises, false).Render(context.Background(), &view); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(view.String(), "/sessions/42?edit=1") {
		t.Fatalf("completed view missing edit link: %s", view.String())
	}
	if strings.Contains(view.String(), "/sets/11/delete") {
		t.Fatalf("read-only completed view unexpectedly allows set deletion: %s", view.String())
	}

	var edit bytes.Buffer
	if err := SessionPage(session, exercises, true).Render(context.Background(), &edit); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/sessions/42/update", `name="performed_on"`, `value="2026-08-24"`, "Save changes", "/sessions/42/sets/11/delete", "2 sets done"} {
		if !strings.Contains(edit.String(), want) {
			t.Errorf("edit view missing %q: %s", want, edit.String())
		}
	}
}

func TestSessionSetPresentation(t *testing.T) {
	if got := setMarker("emom", domain.SessionSet{Position: 4}); got != "4" {
		t.Fatalf("extra EMOM marker=%q want 4", got)
	}
	if got := setMarker("emom", domain.SessionSet{Position: 4, Minute: 7}); got != "7" {
		t.Fatalf("planned EMOM marker=%q want minute 7", got)
	}

	oneSet := []domain.SessionExercise{{ID: 7, SessionID: 42, Name: "Push-up", Mode: "bodyweight", Format: "sets_reps", Sets: []domain.SessionSet{{ID: 11, Position: 1}}}}
	var output bytes.Buffer
	if err := SessionExercises(oneSet, true, true).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "/sets/11/delete") {
		t.Fatalf("single remaining set should not be deletable: %s", output.String())
	}
	for _, want := range []string{`id="exercise-7"`, `hx-target="closest .movement"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("session exercise missing %q: %s", want, output.String())
		}
	}
}

func TestCompletedEMOMMinutesUseActualWork(t *testing.T) {
	exercise := domain.SessionExercise{
		ID: 7, SessionID: 42, Name: "Pull-up", Mode: "bodyweight", Format: "emom", DurationMinutes: 20,
		Sets: []domain.SessionSet{
			{ID: 11, Position: 1, Minute: 1, Reps: 2},
			{ID: 12, Position: 2, Minute: 2, Reps: 2, Skipped: true},
			{ID: 13, Position: 3, Reps: 2},
			{ID: 14, Position: 4},
		},
	}
	if got := completedExerciseSets(exercise); got != 2 {
		t.Fatalf("completed EMOM minutes=%d want 2", got)
	}

	var completed bytes.Buffer
	if err := SessionExercises([]domain.SessionExercise{exercise}, false, false).Render(context.Background(), &completed); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(completed.String(), "2 minutes done") || strings.Contains(completed.String(), "20 planned minutes") {
		t.Fatalf("completed EMOM summary is not based on actual work: %s", completed.String())
	}

	var draft bytes.Buffer
	if err := SessionExercises([]domain.SessionExercise{exercise}, true, true).Render(context.Background(), &draft); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(draft.String(), "20 planned minutes") {
		t.Fatalf("draft EMOM summary should retain planned duration: %s", draft.String())
	}
}

func TestEMOMQuickEntryKeepsPerMinuteFormData(t *testing.T) {
	for _, mode := range []string{"bodyweight", "weighted"} {
		t.Run(mode, func(t *testing.T) {
			exercise := domain.SessionExercise{ID: 7, SessionID: 42, Name: "Pull-up", Mode: mode, Format: "emom", Sets: []domain.SessionSet{
				{ID: 11, Position: 1, Minute: 2, Reps: 8},
				{ID: 12, Position: 2, Minute: 5, Reps: 6, Skipped: true},
			}}
			for _, draft := range []bool{true, false} {
				var output bytes.Buffer
				if err := SessionExercises([]domain.SessionExercise{exercise}, true, draft).Render(context.Background(), &output); err != nil {
					t.Fatal(err)
				}
				html := output.String()
				for _, want := range []string{"data-emom-editor", `name="emom_open_7" checked`, "Apply to all minutes", `form="emom-form-7"`, `name="reps_11"`, `name="reps_12"`, `name="skipped_12"`, ">2</strong>", ">5</strong>", "1 skipped"} {
					if !strings.Contains(html, want) {
						t.Errorf("draft=%v missing %q", draft, want)
					}
				}
				if strings.Contains(html, `name="emom_weight_7"`) != (mode == "weighted") {
					t.Errorf("weight shortcut does not match mode %s", mode)
				}
			}
			var readonly bytes.Buffer
			if err := SessionExercises([]domain.SessionExercise{exercise}, false, false).Render(context.Background(), &readonly); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(readonly.String(), "data-emom-editor") {
				t.Fatal("read-only history contains quick entry")
			}
			exercise.Format = "sets_reps"
			var sets bytes.Buffer
			if err := SessionExercises([]domain.SessionExercise{exercise}, true, true).Render(context.Background(), &sets); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(sets.String(), "data-emom-editor") {
				t.Fatal("sets × reps contains EMOM quick entry")
			}
		})
	}
}

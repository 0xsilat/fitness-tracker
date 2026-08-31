package web

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func TestCardioFormValidation(t *testing.T) {
	valid := cardioForm{ExerciseID: "1", PerformedOn: "2026-08-30", Minutes: "12.5", Notes: "a note"}
	c, err := parseCardioForm(valid)
	if err != nil || c.DurationMinutes != 12.5 || c.Notes != "a note" {
		t.Fatalf("%#v %v", c, err)
	}
	cases := []cardioForm{
		{ExerciseID: "bad", PerformedOn: valid.PerformedOn, Minutes: "1"},
		{ExerciseID: "1", PerformedOn: "2026-02-30", Minutes: "1"},
		{ExerciseID: "1", PerformedOn: valid.PerformedOn, Minutes: "NaN"},
		{ExerciseID: "1", PerformedOn: valid.PerformedOn, Minutes: "12.55"},
	}
	for _, f := range cases {
		if _, err := parseCardioForm(f); err == nil {
			t.Errorf("accepted %#v", f)
		}
	}
}

func TestCardioFormsAndHistoryPresentation(t *testing.T) {
	var b bytes.Buffer
	f := cardioForm{ID: 4, ExerciseID: "2", PerformedOn: "2026-08-30", Minutes: "12.55", Notes: "Keep <this>", Error: "Check duration"}
	items := []domain.Exercise{{ID: 2, Name: "Running", Mode: "cardio", Archived: true}, {ID: 3, Name: "Bench", Mode: "weighted"}}
	if err := CardioFormPage(f, cardioOptions(items, "2")).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`action="/cardio/4/update"`, `value="12.55"`, `role="alert"`, `Keep &lt;this&gt;`, `archived`} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(b.String(), "Bench") {
		t.Fatal("strength exercise offered for cardio")
	}
	if len(cardioOptions(items, "")) != 0 {
		t.Fatal("archived exercise offered for new entry")
	}
	b.Reset()
	entries := []domain.TrainingEntry{{ID: 4, Kind: "cardio", Title: "Running", URL: "/cardio/4", DurationMinutes: 12.5, Status: "completed", PerformedOn: time.Now()}}
	if err := SessionsPage(entries, nil, false, 0, time.Now()).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`href="/cardio/4"`, "12.5", "minutes", "Training log"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(b.String(), "sets done") {
		t.Fatal("cardio displayed as sets")
	}
}

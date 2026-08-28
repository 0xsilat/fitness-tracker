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
	session := domain.Session{ID: 42, WorkoutName: "Lower A", RoutineName: "Hidden routine", Format: "mixed", Status: "completed", PerformedOn: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)}
	var output bytes.Buffer
	if err := SessionRow(session).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{"Lower A", "24 Aug 2026", "completed", "/sessions/42"} {
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

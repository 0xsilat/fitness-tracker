package store

import (
	"testing"
	"time"
)

func TestDecodePrescriptionConvertsLegacyPyramids(t *testing.T) {
	raw := []byte(`{"format":"mixed","movements":[{"exercise_id":1,"exercise_name":"Squat","mode":"weighted","format":"ascending_pyramid","sets":[{"reps":5,"weight_kg":80},{"reps":8,"weight_kg":70}]}]}`)
	p, err := decodePrescription(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Movements[0].Format; got != "sets_reps" {
		t.Fatalf("format=%q", got)
	}
	if p.Movements[0].Sets[0].Reps != 5 || p.Movements[0].Sets[1].WeightKG != 70 {
		t.Fatalf("numeric plan changed: %#v", p.Movements[0].Sets)
	}
}

func TestDecodePrescriptionDefaultsLegacyEMOMInterval(t *testing.T) {
	raw := []byte(`{"movements":[{"exercise_id":1,"exercise_name":"Burpee","mode":"bodyweight","format":"emom","duration_minutes":10,"sets":[{"reps":0,"minute":1}]}]}`)
	p, err := decodePrescription(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Movements[0].IntervalMinutes; got != 1 {
		t.Fatalf("interval=%d", got)
	}
}

func TestDashboardWindowUsesInclusiveCalendarDays(t *testing.T) {
	location := time.FixedZone("Europe/Lisbon", 3600)
	now := time.Date(2026, 8, 28, 19, 45, 0, 0, location)
	window := dashboardWindowFor(now)

	checks := map[string]struct {
		got  time.Time
		want string
	}{
		"today":  {window.Today, "2026-08-28"},
		"seven":  {window.Start7, "2026-08-22"},
		"thirty": {window.Start30, "2026-07-30"},
		"ninety": {window.Start90, "2026-05-31"},
		"year":   {window.Start365, "2025-08-29"},
	}
	for name, check := range checks {
		if got := check.got.Format("2006-01-02"); got != check.want {
			t.Errorf("%s start=%s want %s", name, got, check.want)
		}
		if check.got.Hour() != 0 {
			t.Errorf("%s is not normalized: %v", name, check.got)
		}
	}
	if got := int(window.Today.Sub(window.Start365).Hours()/24) + 1; got != 365 {
		t.Fatalf("activity days=%d want 365", got)
	}
}

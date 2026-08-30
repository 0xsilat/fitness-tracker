package domain

import (
	"math"
	"testing"
	"time"
)

func TestCardioDurationAndWeek(t *testing.T) {
	for _, v := range []float64{0, -1, 12.55, math.NaN(), math.Inf(1), 100000000} {
		if err := ValidateCardio(CardioSession{ExerciseID: 1, PerformedOn: time.Now(), DurationMinutes: v}); err == nil {
			t.Errorf("accepted duration %v", v)
		}
	}
	for _, v := range []float64{0.1, 12.5, 90} {
		if err := ValidateCardio(CardioSession{ExerciseID: 1, PerformedOn: time.Now(), DurationMinutes: v}); err != nil {
			t.Errorf("rejected %v: %v", v, err)
		}
	}
	for _, day := range []string{"2025-12-29", "2026-01-01", "2026-01-04"} {
		d, _ := time.Parse("2006-01-02", day)
		if got := MondayOfWeek(d).Format("2006-01-02"); got != "2025-12-29" {
			t.Errorf("Monday of %s = %s", day, got)
		}
	}
}

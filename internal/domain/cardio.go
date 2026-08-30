package domain

import (
	"errors"
	"math"
	"time"
)

type CardioSession struct {
	ID              int64
	ExerciseID      int64
	ExerciseName    string
	PerformedOn     time.Time
	DurationMinutes float64
	Notes           string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func ValidateCardio(c CardioSession) error {
	if c.ExerciseID <= 0 {
		return errors.New("choose a cardio activity")
	}
	if c.PerformedOn.IsZero() {
		return errors.New("performed date is required")
	}
	d := c.DurationMinutes
	if math.IsNaN(d) || math.IsInf(d, 0) || d <= 0 || d >= 100000000 || math.Abs(d*10-math.Round(d*10)) > 0.000001 {
		return errors.New("enter positive minutes in increments of 0.1 (less than 100,000,000)")
	}
	return nil
}

// TrainingEntry is the shared read model; workout and cardio writes stay separate.
type TrainingEntry struct {
	ID              int64
	Kind            string
	Title           string
	Status          string
	PerformedOn     time.Time
	CompletedSets   int
	DurationMinutes float64
	URL             string
}

type CardioExerciseSummary struct {
	Exercise Exercise
	Sessions int
	Minutes  float64
}

type CardioAnalytics struct {
	From, To   time.Time
	ExerciseID int64
	Sessions   int
	Minutes    float64
	Points     []ChartPoint
	Exercises  []CardioExerciseSummary
}

func MondayOfWeek(day time.Time) time.Time {
	day = time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}

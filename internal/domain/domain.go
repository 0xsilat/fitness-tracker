package domain

import (
	"errors"
	"math"
	"strings"
	"time"
)

type Exercise struct {
	ID       int64
	Name     string
	Mode     string
	Archived bool
}

type PlannedSet struct {
	Reps     int     `json:"reps"`
	WeightKG float64 `json:"weight_kg,omitempty"`
	Minute   int     `json:"minute,omitempty"`
}

type Movement struct {
	ExerciseID      int64        `json:"exercise_id"`
	ExerciseName    string       `json:"exercise_name"`
	Mode            string       `json:"mode"`
	Format          string       `json:"format"`
	DurationMinutes int          `json:"duration_minutes,omitempty"`
	IntervalMinutes int          `json:"interval_minutes,omitempty"`
	Sets            []PlannedSet `json:"sets"`
}

type Prescription struct {
	Format          string     `json:"format"`
	DurationMinutes int        `json:"duration_minutes,omitempty"`
	Movements       []Movement `json:"movements"`
}

type Routine struct {
	ID           int64
	Name         string
	Description  string
	Active       bool
	Archived     bool
	WorkoutCount int
}

type Workout struct {
	ID           int64
	RoutineID    int64
	Name         string
	Position     int
	Format       string
	Prescription Prescription
}

type Session struct {
	ID            int64
	RoutineID     int64
	WorkoutID     *int64
	RoutineName   string
	WorkoutName   string
	Format        string
	Status        string
	PerformedOn   time.Time
	StartedAt     time.Time
	CompletedAt   *time.Time
	Notes         string
	RPE           *int
	CompletedSets int
	Snapshot      Prescription
}

// WorkoutGroup is a routine and its workout templates, used when choosing a
// workout to log. It intentionally includes archived routines so historic
// programs can still be recorded.
type WorkoutGroup struct {
	Routine  Routine
	Workouts []Workout
}

type SessionExercise struct {
	ID              int64
	SessionID       int64
	ExerciseID      int64
	Name            string
	Mode            string
	Format          string
	DurationMinutes int
	IntervalMinutes int
	Position        int
	Sets            []SessionSet
	// PlannedMinutes comes from the session snapshot, not the current workout.
	PlannedMinutes []int
}

type SessionSet struct {
	ID                int64
	SessionExerciseID int64
	Position          int
	Minute            int
	Reps              int
	TargetReps        *int
	WeightKG          float64
	Skipped           bool
	Deleted           bool // Pending form edit; never persisted as a column.
}

type Dashboard struct {
	CardioMinutesThisWeek float64
	ActiveRoutine         *Routine
	Workouts              []Workout
	NextWorkout           *Workout
	Recent                []Session
	Drafts                []Session
	SessionsThisWeek      int
	SessionCount7         int
	SessionCount30        int
	SessionCount90        int
	Activity              []ActivityDay
}

type ActivityDay struct {
	Date     time.Time
	Sessions int
}

type RoutineAnalytics struct {
	Routine         Routine
	From, To        time.Time
	Sessions        int
	Sets            int
	Reps            int
	SessionsPerWeek float64
	LongestGapDays  int
	Points          []ChartPoint
	Exercises       []ExerciseSummary
}

type ExerciseSummary struct {
	Exercise  Exercise
	Sessions  int
	Sets      int
	TotalReps int
	Volume    float64
}

type AnalyticsOverview struct {
	From, To  time.Time
	Exercises []ExerciseSummary
}

type ExerciseAnalytics struct {
	Exercise     Exercise
	From, To     time.Time
	Sessions     int
	TotalReps    int
	Volume       float64
	BestWeight   float64
	BestSetReps  int
	Estimated1RM float64
	Points       []ChartPoint
}

type ChartPoint struct {
	Date  time.Time
	Label string
	Value float64
}

var formats = map[string]bool{"sets_reps": true, "emom": true}

func ValidatePrescription(p Prescription) error {
	for _, movement := range p.Movements {
		if movement.ExerciseID <= 0 || strings.TrimSpace(movement.ExerciseName) == "" {
			return errors.New("exercise is required")
		}
		format := movement.Format
		if format == "" {
			format = p.Format
		}
		duration := movement.DurationMinutes
		if duration == 0 {
			duration = p.DurationMinutes
		}
		if !formats[format] {
			return errors.New("choose a supported format for each exercise")
		}
		if format == "emom" && duration <= 0 {
			return errors.New("EMOM duration must be positive")
		}
		if format == "emom" && movement.IntervalMinutes <= 0 {
			return errors.New("EMOM interval must be positive")
		}
		if len(movement.Sets) == 0 {
			return errors.New("each exercise needs at least one set")
		}
		for _, set := range movement.Sets {
			if set.Reps < 0 {
				return errors.New("target reps cannot be negative")
			}
			if set.WeightKG < 0 {
				return errors.New("weight cannot be negative")
			}
			if format == "emom" && (set.Minute <= 0 || set.Minute > duration) {
				return errors.New("EMOM minute must be within its duration")
			}
		}
	}
	return nil
}

func BuildPlannedSets(format string, setCount, targetReps int, weight float64, startMinute, duration, interval int) ([]PlannedSet, error) {
	if targetReps < 0 {
		return nil, errors.New("target reps cannot be negative")
	}
	if weight < 0 {
		return nil, errors.New("weight cannot be negative")
	}
	var sets []PlannedSet
	switch format {
	case "sets_reps":
		if setCount <= 0 {
			return nil, errors.New("set count must be positive")
		}
		for i := 0; i < setCount; i++ {
			sets = append(sets, PlannedSet{Reps: targetReps, WeightKG: weight})
		}
	case "emom":
		if duration <= 0 {
			return nil, errors.New("EMOM duration must be positive")
		}
		if startMinute <= 0 || startMinute > duration {
			return nil, errors.New("EMOM start minute must be within its duration")
		}
		if interval <= 0 {
			return nil, errors.New("EMOM interval must be positive")
		}
		for minute := startMinute; minute <= duration; minute += interval {
			sets = append(sets, PlannedSet{Reps: targetReps, WeightKG: weight, Minute: minute})
		}
	default:
		return nil, errors.New("choose a supported format")
	}
	return sets, nil
}

func Volume(reps int, weight float64, skipped bool) float64 {
	if skipped || reps < 0 || weight < 0 {
		return 0
	}
	return float64(reps) * weight
}

func Epley1RM(weight float64, reps int) float64 {
	if weight <= 0 || reps <= 0 {
		return 0
	}
	return math.Round((weight*(1+float64(reps)/30))*10) / 10
}

func NextWorkout(workouts []Workout, lastWorkoutID int64) *Workout {
	if len(workouts) == 0 {
		return nil
	}
	for i := range workouts {
		if workouts[i].ID == lastWorkoutID {
			return &workouts[(i+1)%len(workouts)]
		}
	}
	return &workouts[0]
}

package domain

import "testing"

func TestPrescriptionValidation(t *testing.T) {
	valid := Prescription{Movements: []Movement{{ExerciseID: 1, ExerciseName: "Squat", Format: "sets_reps", Sets: []PlannedSet{{Reps: 0}, {Reps: 8}}}}}
	if err := ValidatePrescription(valid); err != nil {
		t.Fatalf("valid prescription: %v", err)
	}
	valid.Movements[0].Sets[0].Reps = -1
	if err := ValidatePrescription(valid); err == nil {
		t.Fatal("expected negative target error")
	}
}

func TestMixedFormatPrescription(t *testing.T) {
	p := Prescription{Movements: []Movement{
		{ExerciseID: 1, ExerciseName: "Squat", Format: "sets_reps", Sets: []PlannedSet{{Reps: 5}, {Reps: 5}}},
		{ExerciseID: 2, ExerciseName: "Burpee", Format: "emom", DurationMinutes: 10, IntervalMinutes: 1, Sets: []PlannedSet{{Reps: 8, Minute: 1}, {Reps: 8, Minute: 2}}},
	}}
	if err := ValidatePrescription(p); err != nil {
		t.Fatalf("mixed prescription: %v", err)
	}
	p.Movements[1].DurationMinutes = 1
	if err := ValidatePrescription(p); err == nil {
		t.Fatal("expected EMOM minute range error")
	}
}

func TestBuildPlannedSets(t *testing.T) {
	sets, err := BuildPlannedSets("sets_reps", 3, 0, 0, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 3 || sets[0].Reps != 0 {
		t.Fatalf("sets=%#v", sets)
	}
	emom, err := BuildPlannedSets("emom", 0, 8, 0, 2, 10, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(emom) != 3 || emom[0].Minute != 2 || emom[2].Minute != 8 {
		t.Fatalf("emom=%#v", emom)
	}
	if _, err := BuildPlannedSets("emom", 0, 0, 0, 1, 10, 0); err == nil {
		t.Fatal("expected interval error")
	}
	if _, err := BuildPlannedSets("ascending_pyramid", 3, 5, 0, 0, 0, 0); err == nil {
		t.Fatal("expected removed format error")
	}
}

func TestCalculations(t *testing.T) {
	if got := Volume(8, 100, false); got != 800 {
		t.Fatalf("volume = %v", got)
	}
	if got := Epley1RM(100, 5); got != 116.7 {
		t.Fatalf("1RM = %v", got)
	}
	if got := Volume(8, 100, true); got != 0 {
		t.Fatalf("skipped volume = %v", got)
	}
}

func TestNextWorkout(t *testing.T) {
	workouts := []Workout{{ID: 1}, {ID: 2}, {ID: 3}}
	if got := NextWorkout(workouts, 2); got == nil || got.ID != 3 {
		t.Fatalf("next = %#v", got)
	}
	if got := NextWorkout(workouts, 3); got == nil || got.ID != 1 {
		t.Fatalf("wrapped next = %#v", got)
	}
	if got := NextWorkout(workouts, 99); got == nil || got.ID != 1 {
		t.Fatalf("deleted workout fallback = %#v", got)
	}
}

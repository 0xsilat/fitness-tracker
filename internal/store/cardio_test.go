package store

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/local/fitness-tracker/internal/domain"
)

// Each integration test owns a schema, never application tables or user data.
func cardioTestStore(t *testing.T, migrate bool) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("cardio_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	s := New(pool)
	if migrate {
		if err = s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func TestCardioIntegration(t *testing.T) {
	s := cardioTestStore(t, true)
	ctx := context.Background()
	var running, cycling, weighted int64
	for name, target := range map[string]*int64{"Running": &running, "Cycling": &cycling, "Bench Press": &weighted} {
		if err := s.pool.QueryRow(ctx, `SELECT id FROM exercises WHERE name=$1`, name).Scan(target); err != nil {
			t.Fatal(err)
		}
	}
	save := func(exercise int64, day string, minutes float64) int64 {
		t.Helper()
		d, _ := time.Parse("2006-01-02", day)
		id, err := s.SaveCardio(ctx, domain.CardioSession{ExerciseID: exercise, PerformedOn: d, DurationMinutes: minutes})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	first := save(running, "2025-12-31", 12.5)
	save(cycling, "2026-01-04", 20)
	save(running, "2026-01-05", 30)
	save(running, "2026-01-19", 10)
	a, err := s.CardioAnalytics(ctx, "2026-01-01", "2026-01-20", 0)
	if err != nil {
		t.Fatal(err)
	}
	if a.Sessions != 3 || a.Minutes != 60 || len(a.Points) != 4 {
		t.Fatalf("analytics: %#v", a)
	}
	for i, want := range []float64{20, 30, 0, 10} {
		if a.Points[i].Value != want {
			t.Errorf("week %d=%v want %v", i, a.Points[i].Value, want)
		}
	}
	filtered, err := s.CardioAnalytics(ctx, "2025-12-31", "2025-12-31", running)
	if err != nil || filtered.Minutes != 12.5 || len(filtered.Points) != 1 {
		t.Fatalf("inclusive single-day filter: %#v %v", filtered, err)
	}
	c, err := s.CardioSession(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	c.ExerciseID = weighted
	if _, err = s.SaveCardio(ctx, c); err == nil {
		t.Fatal("accepted strength exercise")
	}
	c.ExerciseID = running
	if err = s.ArchiveExercise(ctx, running); err != nil {
		t.Fatal(err)
	}
	c.DurationMinutes = 15.5
	c.Notes = "edited"
	if _, err = s.SaveCardio(ctx, c); err != nil {
		t.Fatal(err)
	}
	c.ID = 0
	if _, err = s.SaveCardio(ctx, c); err == nil {
		t.Fatal("accepted new archived activity")
	}
	c.ID = first
	if err = s.DeleteCardio(ctx, first); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CardioSession(ctx, first); err != ErrNotFound {
		t.Fatalf("deleted entry error: %v", err)
	}
	filtered, err = s.CardioAnalytics(ctx, "2025-12-31", "2025-12-31", running)
	if err != nil || filtered.Minutes != 0 || filtered.Sessions != 0 {
		t.Fatalf("deleted entry still counted: %#v %v", filtered, err)
	}
	if err = s.CreateExercise(ctx, "Stair climber", "cardio"); err != nil {
		t.Fatal(err)
	}
	// Clear only this test's historical fixtures, making current-day assertions
	// independent of the calendar date on which the suite is run.
	history, err := s.TrainingLog(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range history {
		if err = s.DeleteCardio(ctx, entry.ID); err != nil {
			t.Fatal(err)
		}
	}

	// A workout and two cardio entries on the same day count as three sessions.
	today := time.Now().Format("2006-01-02")
	save(cycling, today, 12.5)
	save(cycling, today, 7.5)
	var routine, session int64
	if err = s.pool.QueryRow(ctx, `INSERT INTO routines(name,active) VALUES('Test routine',true) RETURNING id`).Scan(&routine); err != nil {
		t.Fatal(err)
	}
	if err = s.pool.QueryRow(ctx, `INSERT INTO sessions(routine_id,routine_name,workout_name,format,status,performed_on,prescription_snapshot,completed_at) VALUES($1,'Test routine','Workout','mixed','completed',$2,'{"movements":[]}',now()) RETURNING id`, routine, today).Scan(&session); err != nil {
		t.Fatal(err)
	}
	d, err := s.Dashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if d.CardioMinutesThisWeek != 20 || d.SessionCount7 != 3 || d.Activity[len(d.Activity)-1].Sessions != 3 {
		t.Fatalf("dashboard: minutes=%v sessions=%d day=%#v", d.CardioMinutesThisWeek, d.SessionCount7, d.Activity[len(d.Activity)-1])
	}
	log, err := s.TrainingLog(ctx, 2)
	if err != nil || len(log) != 2 || log[0].Kind != "workout" || log[1].Kind != "cardio" {
		t.Fatalf("combined limit/order: %#v %v", log, err)
	}
	ra, err := s.RoutineAnalytics(ctx, routine, today, today)
	if err != nil || ra.Sessions != 1 {
		t.Fatalf("routine analytics included cardio: %#v %v", ra, err)
	}
	overview, err := s.AnalyticsOverview(ctx, today, today)
	if err != nil || len(overview.Exercises) != 0 {
		t.Fatalf("strength overview included cardio: %#v %v", overview, err)
	}
	var workout int64
	if err = s.pool.QueryRow(ctx, `INSERT INTO workouts(routine_id,name,position,format,prescription) VALUES($1,'Plan',1,'mixed','{"movements":[]}') RETURNING id`, routine).Scan(&workout); err != nil {
		t.Fatal(err)
	}
	if err = s.AddMovement(ctx, workout, cycling, "sets_reps", 3, 10, 0, 0, 0, 0); err == nil {
		t.Fatal("accepted cardio in a workout plan")
	}
}

func TestCardioMigrationPreservesExistingExercises(t *testing.T) {
	s := cardioTestStore(t, false)
	ctx := context.Background()
	for _, name := range []string{"001_init.sql", "002_movement_formats.sql", "003_optional_targets.sql", "004_delete_workout_preserve_history.sql", "005_session_performed_on.sql"} {
		body, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, string(body)); err != nil {
			t.Fatal(err)
		}
		if _, err = s.pool.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, name); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.CreateExercise(ctx, "Treadmill", "weighted"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var mode string
	if err := s.pool.QueryRow(ctx, `SELECT mode FROM exercises WHERE name='Treadmill'`).Scan(&mode); err != nil || mode != "weighted" {
		t.Fatalf("existing exercise changed: %s %v", mode, err)
	}
	var count int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM exercises WHERE mode='cardio'`).Scan(&count); err != nil || count != 9 {
		t.Fatalf("seeds: %d %v", count, err)
	}
}

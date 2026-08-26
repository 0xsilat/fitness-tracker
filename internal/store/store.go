package store

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/local/fitness-tracker/internal/domain"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

var ErrNotFound = errors.New("not found")

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) Migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return err
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		var exists bool
		if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)`, entry.Name()).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		tx, err := s.pool.Begin(ctx)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, string(body)); err == nil {
			_, err = tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES($1)`, entry.Name())
		}
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", entry.Name(), err)
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Exercises(ctx context.Context, includeArchived bool) ([]domain.Exercise, error) {
	query := `SELECT id,name,mode,archived FROM exercises`
	if !includeArchived {
		query += ` WHERE NOT archived`
	}
	query += ` ORDER BY archived,name`
	rows, err := s.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Exercise
	for rows.Next() {
		var x domain.Exercise
		if err := rows.Scan(&x.ID, &x.Name, &x.Mode, &x.Archived); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) Exercise(ctx context.Context, id int64) (domain.Exercise, error) {
	var x domain.Exercise
	err := s.pool.QueryRow(ctx, `SELECT id,name,mode,archived FROM exercises WHERE id=$1`, id).Scan(&x.ID, &x.Name, &x.Mode, &x.Archived)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return x, err
}

func (s *Store) CreateExercise(ctx context.Context, name, mode string) error {
	name = strings.TrimSpace(name)
	if name == "" || (mode != "weighted" && mode != "bodyweight") {
		return errors.New("name and valid tracking mode are required")
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO exercises(name,mode) VALUES($1,$2)`, name, mode)
	return err
}

func (s *Store) ArchiveExercise(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE exercises SET archived=true WHERE id=$1`, id)
	return err
}

func decodePrescription(raw []byte) (domain.Prescription, error) {
	var p domain.Prescription
	if err := json.Unmarshal(raw, &p); err != nil {
		return p, err
	}
	for i := range p.Movements {
		if p.Movements[i].Format == "" {
			p.Movements[i].Format = p.Format
		}
		if p.Movements[i].Format == "ascending_pyramid" || p.Movements[i].Format == "descending_pyramid" {
			p.Movements[i].Format = "sets_reps"
		}
		if p.Movements[i].DurationMinutes == 0 && p.Movements[i].Format == "emom" {
			p.Movements[i].DurationMinutes = p.DurationMinutes
		}
		if p.Movements[i].Format == "emom" && p.Movements[i].IntervalMinutes == 0 {
			p.Movements[i].IntervalMinutes = 1
		}
	}
	return p, nil
}
func encodePrescription(p domain.Prescription) ([]byte, error) { return json.Marshal(p) }

func (s *Store) Routines(ctx context.Context) ([]domain.Routine, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id,r.name,r.description,r.active,r.archived,count(w.id) FROM routines r LEFT JOIN workouts w ON w.routine_id=r.id GROUP BY r.id ORDER BY r.archived,r.active DESC,r.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Routine
	for rows.Next() {
		var r domain.Routine
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Active, &r.Archived, &r.WorkoutCount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) Routine(ctx context.Context, id int64) (domain.Routine, error) {
	var r domain.Routine
	err := s.pool.QueryRow(ctx, `SELECT r.id,r.name,r.description,r.active,r.archived,count(w.id) FROM routines r LEFT JOIN workouts w ON w.routine_id=r.id WHERE r.id=$1 GROUP BY r.id`, id).Scan(&r.ID, &r.Name, &r.Description, &r.Active, &r.Archived, &r.WorkoutCount)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return r, err
}

func (s *Store) CreateRoutine(ctx context.Context, name, description string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, errors.New("routine name is required")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var any bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM routines WHERE active)`).Scan(&any); err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO routines(name,description,active) VALUES($1,$2,$3) RETURNING id`, name, strings.TrimSpace(description), !any).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit(ctx)
}

func (s *Store) ActivateRoutine(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `UPDATE routines SET active=false WHERE active`); err != nil {
		return err
	}
	result, err := tx.Exec(ctx, `UPDATE routines SET active=true,archived=false,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

func (s *Store) ArchiveRoutine(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE routines SET archived=true,active=false,updated_at=now() WHERE id=$1`, id)
	return err
}

func (s *Store) DuplicateRoutine(ctx context.Context, id int64) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var newID int64
	err = tx.QueryRow(ctx, `INSERT INTO routines(name,description) SELECT name || ' copy',description FROM routines WHERE id=$1 RETURNING id`, id).Scan(&newID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workouts(routine_id,name,position,format,prescription) SELECT $1,name,position,format,prescription FROM workouts WHERE routine_id=$2`, newID, id)
	if err != nil {
		return 0, err
	}
	return newID, tx.Commit(ctx)
}

func (s *Store) Workouts(ctx context.Context, routineID int64) ([]domain.Workout, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,routine_id,name,position,format,prescription FROM workouts WHERE routine_id=$1 ORDER BY position`, routineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Workout
	for rows.Next() {
		var w domain.Workout
		var raw []byte
		if err := rows.Scan(&w.ID, &w.RoutineID, &w.Name, &w.Position, &w.Format, &raw); err != nil {
			return nil, err
		}
		w.Prescription, err = decodePrescription(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) Workout(ctx context.Context, id int64) (domain.Workout, error) {
	var w domain.Workout
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT id,routine_id,name,position,format,prescription FROM workouts WHERE id=$1`, id).Scan(&w.ID, &w.RoutineID, &w.Name, &w.Position, &w.Format, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return w, ErrNotFound
	}
	if err == nil {
		w.Prescription, err = decodePrescription(raw)
	}
	return w, err
}

func (s *Store) CreateWorkout(ctx context.Context, routineID int64, name string) (int64, error) {
	p := domain.Prescription{Format: "mixed", Movements: []domain.Movement{}}
	raw, _ := encodePrescription(p)
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO workouts(routine_id,name,position,format,prescription) VALUES($1,$2,(SELECT coalesce(max(position),0)+1 FROM workouts WHERE routine_id=$1),'mixed',$3) RETURNING id`, routineID, strings.TrimSpace(name), raw).Scan(&id)
	return id, err
}

func (s *Store) AddMovement(ctx context.Context, workoutID, exerciseID int64, format string, setCount, targetReps int, weight float64, startMinute, duration, interval int) error {
	w, err := s.Workout(ctx, workoutID)
	if err != nil {
		return err
	}
	exercise, err := s.Exercise(ctx, exerciseID)
	if err != nil {
		return err
	}
	sets, err := domain.BuildPlannedSets(format, setCount, targetReps, weight, startMinute, duration, interval)
	if err != nil {
		return err
	}
	m := domain.Movement{ExerciseID: exercise.ID, ExerciseName: exercise.Name, Mode: exercise.Mode, Format: format, DurationMinutes: duration, IntervalMinutes: interval, Sets: sets}
	w.Prescription.Movements = append(w.Prescription.Movements, m)
	if err := domain.ValidatePrescription(w.Prescription); err != nil {
		return err
	}
	raw, _ := encodePrescription(w.Prescription)
	_, err = s.pool.Exec(ctx, `UPDATE workouts SET prescription=$2,updated_at=now() WHERE id=$1`, workoutID, raw)
	return err
}

func (s *Store) RemoveMovement(ctx context.Context, workoutID int64, index int) error {
	w, err := s.Workout(ctx, workoutID)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(w.Prescription.Movements) {
		return ErrNotFound
	}
	w.Prescription.Movements = append(w.Prescription.Movements[:index], w.Prescription.Movements[index+1:]...)
	raw, _ := encodePrescription(w.Prescription)
	_, err = s.pool.Exec(ctx, `UPDATE workouts SET prescription=$2,updated_at=now() WHERE id=$1`, workoutID, raw)
	return err
}

func (s *Store) MoveWorkout(ctx context.Context, id int64, direction int) error {
	w, err := s.Workout(ctx, id)
	if err != nil {
		return err
	}
	target := w.Position + direction
	if target < 1 {
		return nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var other int64
	err = tx.QueryRow(ctx, `SELECT id FROM workouts WHERE routine_id=$1 AND position=$2`, w.RoutineID, target).Scan(&other)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workouts SET position=0 WHERE id=$1`, w.ID); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workouts SET position=$2 WHERE id=$1`, other, w.Position); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workouts SET position=$2 WHERE id=$1`, w.ID, target); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) DeleteWorkout(ctx context.Context, id int64) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var routineID int64
	var position int
	err = tx.QueryRow(ctx, `SELECT routine_id,position FROM workouts WHERE id=$1 FOR UPDATE`, id).Scan(&routineID, &position)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM sessions WHERE workout_id=$1 AND status='draft'`, id); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `DELETE FROM workouts WHERE id=$1`, id); err != nil {
		return err
	}
	var maxPosition int
	if err = tx.QueryRow(ctx, `SELECT coalesce(max(position),0) FROM workouts WHERE routine_id=$1`, routineID).Scan(&maxPosition); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workouts SET position=position+$3,updated_at=now() WHERE routine_id=$1 AND position>$2`, routineID, position, maxPosition); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE workouts SET position=position-$2-1 WHERE routine_id=$1 AND position>$2`, routineID, maxPosition); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE routines SET updated_at=now() WHERE id=$1`, routineID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func parseDateRange(fromText, toText string) (time.Time, time.Time, error) {
	now := time.Now()
	from := now.AddDate(0, 0, -90)
	to := now
	var err error
	if fromText != "" {
		from, err = time.Parse("2006-01-02", fromText)
		if err != nil {
			return from, to, err
		}
	}
	if toText != "" {
		to, err = time.Parse("2006-01-02", toText)
		if err != nil {
			return from, to, err
		}
		to = to.Add(24*time.Hour - time.Nanosecond)
	}
	if from.After(to) {
		return from, to, errors.New("start date must be before end date")
	}
	return from, to, nil
}

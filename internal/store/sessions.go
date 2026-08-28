package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/local/fitness-tracker/internal/domain"
)

func (s *Store) StartSession(ctx context.Context, workoutID int64, performedOn time.Time) (int64, error) {
	var existing int64
	err := s.pool.QueryRow(ctx, `SELECT id FROM sessions WHERE workout_id=$1 AND status='draft'`, workoutID).Scan(&existing)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	w, err := s.Workout(ctx, workoutID)
	if err != nil {
		return 0, err
	}
	r, err := s.Routine(ctx, w.RoutineID)
	if err != nil {
		return 0, err
	}
	raw, _ := encodePrescription(w.Prescription)
	if len(w.Prescription.Movements) == 0 {
		return 0, errors.New("add at least one exercise before starting this workout")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var sessionID int64
	err = tx.QueryRow(ctx, `INSERT INTO sessions(routine_id,workout_id,routine_name,workout_name,format,status,performed_on,prescription_snapshot) VALUES($1,$2,$3,$4,'mixed','draft',$5,$6) RETURNING id`, r.ID, w.ID, r.Name, w.Name, performedOn, raw).Scan(&sessionID)
	if err != nil {
		return 0, err
	}
	for i, m := range w.Prescription.Movements {
		var exerciseID int64
		err = tx.QueryRow(ctx, `INSERT INTO session_exercises(session_id,exercise_id,exercise_name,tracking_mode,workout_format,duration_minutes,interval_minutes,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id`, sessionID, m.ExerciseID, m.ExerciseName, m.Mode, m.Format, m.DurationMinutes, m.IntervalMinutes, i+1).Scan(&exerciseID)
		if err != nil {
			return 0, err
		}
		for j, set := range m.Sets {
			if _, err = tx.Exec(ctx, `INSERT INTO session_sets(session_exercise_id,position,planned_minute,reps,target_reps,weight_kg) VALUES($1,$2,$3,$4,NULLIF($4,0),$5)`, exerciseID, j+1, set.Minute, set.Reps, set.WeightKG); err != nil {
				return 0, err
			}
		}
	}
	return sessionID, tx.Commit(ctx)
}

func (s *Store) Session(ctx context.Context, id int64) (domain.Session, []domain.SessionExercise, error) {
	var out domain.Session
	var raw []byte
	err := s.pool.QueryRow(ctx, `SELECT id,routine_id,workout_id,routine_name,workout_name,format,status,performed_on,started_at,completed_at,notes,rpe,prescription_snapshot FROM sessions WHERE id=$1`, id).Scan(&out.ID, &out.RoutineID, &out.WorkoutID, &out.RoutineName, &out.WorkoutName, &out.Format, &out.Status, &out.PerformedOn, &out.StartedAt, &out.CompletedAt, &out.Notes, &out.RPE, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil, ErrNotFound
	}
	if err != nil {
		return out, nil, err
	}
	if out.Snapshot, err = decodePrescription(raw); err != nil {
		return out, nil, err
	}
	rows, err := s.pool.Query(ctx, `SELECT se.id,se.session_id,se.exercise_id,se.exercise_name,se.tracking_mode,se.workout_format,se.duration_minutes,se.interval_minutes,se.position,ss.id,ss.position,ss.planned_minute,ss.reps,ss.target_reps,ss.weight_kg,ss.skipped FROM session_exercises se LEFT JOIN session_sets ss ON ss.session_exercise_id=se.id WHERE se.session_id=$1 ORDER BY se.position,ss.position`, id)
	if err != nil {
		return out, nil, err
	}
	defer rows.Close()
	var exercises []domain.SessionExercise
	var current *domain.SessionExercise
	for rows.Next() {
		var e domain.SessionExercise
		var setID *int64
		var setPosition, minute, reps *int
		var targetReps *int
		var weight *float64
		var skipped *bool
		if err := rows.Scan(&e.ID, &e.SessionID, &e.ExerciseID, &e.Name, &e.Mode, &e.Format, &e.DurationMinutes, &e.IntervalMinutes, &e.Position, &setID, &setPosition, &minute, &reps, &targetReps, &weight, &skipped); err != nil {
			return out, nil, err
		}
		if current == nil || current.ID != e.ID {
			exercises = append(exercises, e)
			current = &exercises[len(exercises)-1]
		}
		if setID != nil {
			current.Sets = append(current.Sets, domain.SessionSet{ID: *setID, SessionExerciseID: e.ID, Position: *setPosition, Minute: *minute, Reps: *reps, TargetReps: targetReps, WeightKG: *weight, Skipped: *skipped})
		}
	}
	return out, exercises, rows.Err()
}

type SetUpdate struct {
	ID       int64
	Reps     int
	WeightKG float64
	Skipped  bool
}

func (s *Store) SaveDraft(ctx context.Context, id int64, notes string, rpe *int, updates []SetUpdate) error {
	if rpe != nil && (*rpe < 1 || *rpe > 10) {
		return errors.New("effort must be between 1 and 10")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	result, err := tx.Exec(ctx, `UPDATE sessions SET notes=$2,rpe=$3 WHERE id=$1 AND status='draft'`, id, notes, rpe)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("completed sessions cannot be edited")
	}
	for _, u := range updates {
		if u.Reps < 0 || u.WeightKG < 0 {
			return errors.New("reps and weight cannot be negative")
		}
		if _, err = tx.Exec(ctx, `UPDATE session_sets ss SET reps=$2,weight_kg=$3,skipped=$4 FROM session_exercises se WHERE ss.id=$1 AND ss.session_exercise_id=se.id AND se.session_id=$5`, u.ID, u.Reps, u.WeightKG, u.Skipped, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Store) AddSessionSet(ctx context.Context, sessionID, sessionExerciseID int64) error {
	result, err := s.pool.Exec(ctx, `INSERT INTO session_sets(session_exercise_id,position,reps,weight_kg) SELECT se.id,coalesce(max(ss.position),0)+1,0,0 FROM session_exercises se LEFT JOIN session_sets ss ON ss.session_exercise_id=se.id JOIN sessions s ON s.id=se.session_id WHERE se.id=$2 AND s.id=$1 AND s.status='draft' GROUP BY se.id`, sessionID, sessionExerciseID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CompleteSession(ctx context.Context, id int64) error {
	var unfinished int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM session_sets ss JOIN session_exercises se ON se.id=ss.session_exercise_id JOIN sessions s ON s.id=se.session_id WHERE s.id=$1 AND s.status='draft' AND NOT ss.skipped AND ss.reps<=0`, id).Scan(&unfinished); err != nil {
		return err
	}
	if unfinished > 0 {
		return errors.New("enter actual reps or mark every unfinished set as skipped")
	}
	result, err := s.pool.Exec(ctx, `UPDATE sessions SET status='completed',completed_at=now() WHERE id=$1 AND status='draft'`, id)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return errors.New("session is already completed")
	}
	return nil
}

func (s *Store) DiscardSession(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND status='draft'`, id)
	return err
}
func (s *Store) DeleteSession(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1 AND status='completed'`, id)
	return err
}

func (s *Store) Sessions(ctx context.Context, limit int) ([]domain.Session, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id,routine_id,workout_id,routine_name,workout_name,format,status,performed_on,started_at,completed_at,notes,rpe FROM sessions ORDER BY performed_on DESC,CASE WHEN status='draft' THEN 0 ELSE 1 END,coalesce(completed_at,started_at) DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Session
	for rows.Next() {
		var x domain.Session
		if err := rows.Scan(&x.ID, &x.RoutineID, &x.WorkoutID, &x.RoutineName, &x.WorkoutName, &x.Format, &x.Status, &x.PerformedOn, &x.StartedAt, &x.CompletedAt, &x.Notes, &x.RPE); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) WorkoutGroups(ctx context.Context) ([]domain.WorkoutGroup, error) {
	routines, err := s.Routines(ctx)
	if err != nil {
		return nil, err
	}
	groups := make([]domain.WorkoutGroup, 0, len(routines))
	for _, routine := range routines {
		workouts, err := s.Workouts(ctx, routine.ID)
		if err != nil {
			return nil, err
		}
		if len(workouts) > 0 {
			groups = append(groups, domain.WorkoutGroup{Routine: routine, Workouts: workouts})
		}
	}
	return groups, nil
}

type dashboardWindow struct {
	Today, Start7, Start30, Start90, Start365 time.Time
}

func dashboardWindowFor(now time.Time) dashboardWindow {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return dashboardWindow{
		Today:    today,
		Start7:   today.AddDate(0, 0, -6),
		Start30:  today.AddDate(0, 0, -29),
		Start90:  today.AddDate(0, 0, -89),
		Start365: today.AddDate(0, 0, -364),
	}
}

func (s *Store) Dashboard(ctx context.Context) (domain.Dashboard, error) {
	var d domain.Dashboard
	var activeID int64
	err := s.pool.QueryRow(ctx, `SELECT id,name,description,active,archived FROM routines WHERE active`).Scan(&activeID, new(string), new(string), new(bool), new(bool))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return d, err
	}
	if err == nil {
		r, e := s.Routine(ctx, activeID)
		if e != nil {
			return d, e
		}
		d.ActiveRoutine = &r
		d.Workouts, e = s.Workouts(ctx, activeID)
		if e != nil {
			return d, e
		}
		var last int64
		_ = s.pool.QueryRow(ctx, `SELECT s.workout_id FROM sessions s JOIN workouts w ON w.id=s.workout_id WHERE s.routine_id=$1 AND s.status='completed' ORDER BY s.performed_on DESC,s.completed_at DESC LIMIT 1`, activeID).Scan(&last)
		d.NextWorkout = domain.NextWorkout(d.Workouts, last)
	}
	all, err := s.Sessions(ctx, 100)
	if err != nil {
		return d, err
	}
	for _, session := range all {
		if session.Status == "draft" {
			d.Drafts = append(d.Drafts, session)
		} else {
			d.Recent = append(d.Recent, session)
		}
	}
	window := dashboardWindowFor(time.Now())
	err = s.pool.QueryRow(ctx, `SELECT
		count(*) FILTER (WHERE performed_on BETWEEN $2::date AND $1::date),
		count(*) FILTER (WHERE performed_on BETWEEN $3::date AND $1::date),
		count(*) FILTER (WHERE performed_on BETWEEN $4::date AND $1::date)
		FROM sessions WHERE status='completed'`, window.Today, window.Start7, window.Start30, window.Start90).Scan(&d.SessionCount7, &d.SessionCount30, &d.SessionCount90)
	if err != nil {
		return d, err
	}
	d.SessionsThisWeek = d.SessionCount7
	rows, err := s.pool.Query(ctx, `SELECT days.day, count(s.id)
		FROM generate_series($2::date, $1::date, interval '1 day') AS days(day)
		LEFT JOIN sessions s ON s.status='completed' AND s.performed_on=days.day
		GROUP BY days.day ORDER BY days.day`, window.Today, window.Start365)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var day domain.ActivityDay
		if err = rows.Scan(&day.Date, &day.Sessions); err != nil {
			return d, err
		}
		d.Activity = append(d.Activity, day)
	}
	if err = rows.Err(); err != nil {
		return d, err
	}
	return d, nil
}

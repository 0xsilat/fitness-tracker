package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/local/fitness-tracker/internal/domain"
)

func (s *Store) CardioSession(ctx context.Context, id int64) (domain.CardioSession, error) {
	var c domain.CardioSession
	err := s.pool.QueryRow(ctx, `SELECT id,exercise_id,exercise_name,performed_on,duration_minutes,notes,created_at,updated_at FROM cardio_sessions WHERE id=$1`, id).Scan(&c.ID, &c.ExerciseID, &c.ExerciseName, &c.PerformedOn, &c.DurationMinutes, &c.Notes, &c.CreatedAt, &c.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrNotFound
	}
	return c, err
}

// SaveCardio validates the exercise under a lock so concurrent archival cannot
// introduce a newly selected archived activity. Existing selections remain valid.
func (s *Store) SaveCardio(ctx context.Context, c domain.CardioSession) (int64, error) {
	if err := domain.ValidateCardio(c); err != nil {
		return 0, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	var previousID int64
	var previousName string
	if c.ID != 0 {
		err = tx.QueryRow(ctx, `SELECT exercise_id,exercise_name FROM cardio_sessions WHERE id=$1 FOR UPDATE`, c.ID).Scan(&previousID, &previousName)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, ErrNotFound
		}
		if err != nil {
			return 0, err
		}
	}
	var name, mode string
	var archived bool
	err = tx.QueryRow(ctx, `SELECT name,mode,archived FROM exercises WHERE id=$1 FOR SHARE`, c.ExerciseID).Scan(&name, &mode, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("choose a cardio activity")
	}
	if err != nil {
		return 0, err
	}
	if mode != "cardio" || (archived && previousID != c.ExerciseID) {
		return 0, errors.New("choose an active cardio activity")
	}
	if c.ID == 0 {
		err = tx.QueryRow(ctx, `INSERT INTO cardio_sessions(exercise_id,exercise_name,performed_on,duration_minutes,notes) VALUES($1,$2,$3,$4,$5) RETURNING id`, c.ExerciseID, name, c.PerformedOn, c.DurationMinutes, c.Notes).Scan(&c.ID)
	} else {
		if previousID == c.ExerciseID {
			name = previousName
		}
		_, err = tx.Exec(ctx, `UPDATE cardio_sessions SET exercise_id=$2,exercise_name=$3,performed_on=$4,duration_minutes=$5,notes=$6,updated_at=now() WHERE id=$1`, c.ID, c.ExerciseID, name, c.PerformedOn, c.DurationMinutes, c.Notes)
	}
	if err != nil {
		return 0, err
	}
	return c.ID, tx.Commit(ctx)
}

func (s *Store) DeleteCardio(ctx context.Context, id int64) error {
	result, err := s.pool.Exec(ctx, `DELETE FROM cardio_sessions WHERE id=$1`, id)
	if err == nil && result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) TrainingLog(ctx context.Context, limit int) ([]domain.TrainingEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.pool.Query(ctx, `SELECT id,kind,title,status,performed_on,sets,minutes FROM (
 SELECT s.id,'workout' AS kind,s.workout_name AS title,s.status,s.performed_on,
 (SELECT count(*) FROM session_exercises se JOIN session_sets ss ON ss.session_exercise_id=se.id WHERE se.session_id=s.id AND ss.reps>0 AND NOT ss.skipped) AS sets,
 0::numeric AS minutes,coalesce(s.completed_at,s.started_at) AS recorded_at FROM sessions s
 UNION ALL
 SELECT id,'cardio',exercise_name,'completed',performed_on,0,duration_minutes,created_at FROM cardio_sessions
 ) entries ORDER BY performed_on DESC,CASE WHEN status='draft' THEN 0 ELSE 1 END,recorded_at DESC,kind,id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []domain.TrainingEntry
	for rows.Next() {
		var x domain.TrainingEntry
		if err = rows.Scan(&x.ID, &x.Kind, &x.Title, &x.Status, &x.PerformedOn, &x.CompletedSets, &x.DurationMinutes); err != nil {
			return nil, err
		}
		prefix := "sessions"
		if x.Kind == "cardio" {
			prefix = "cardio"
		}
		x.URL = fmt.Sprintf("/%s/%d", prefix, x.ID)
		entries = append(entries, x)
	}
	return entries, rows.Err()
}

func (s *Store) CardioAnalytics(ctx context.Context, fromText, toText string, exerciseID int64) (domain.CardioAnalytics, error) {
	from, to, err := parseDateRange(fromText, toText)
	a := domain.CardioAnalytics{From: from, To: to, ExerciseID: exerciseID}
	if err != nil {
		return a, err
	}
	if exerciseID != 0 {
		e, err := s.Exercise(ctx, exerciseID)
		if err != nil {
			return a, err
		}
		if e.Mode != "cardio" {
			return a, errors.New("choose a cardio activity")
		}
	}
	rows, err := s.pool.Query(ctx, `SELECT e.id,e.name,e.mode,e.archived,count(*),sum(c.duration_minutes)
 FROM cardio_sessions c JOIN exercises e ON e.id=c.exercise_id
 WHERE c.performed_on BETWEEN $1::date AND $2::date AND ($3::bigint=0 OR c.exercise_id=$3)
 GROUP BY e.id ORDER BY sum(c.duration_minutes) DESC,e.name`, from, to, exerciseID)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var x domain.CardioExerciseSummary
		if err = rows.Scan(&x.Exercise.ID, &x.Exercise.Name, &x.Exercise.Mode, &x.Exercise.Archived, &x.Sessions, &x.Minutes); err != nil {
			rows.Close()
			return a, err
		}
		a.Sessions += x.Sessions
		a.Minutes += x.Minutes
		a.Exercises = append(a.Exercises, x)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return a, err
	}
	rows, err = s.pool.Query(ctx, `WITH weeks AS (
 SELECT generate_series(date_trunc('week',$1::date::timestamp),date_trunc('week',$2::date::timestamp),interval '1 week') AS start
 ), totals AS (
 SELECT date_trunc('week',performed_on::timestamp) AS start,sum(duration_minutes) AS minutes
 FROM cardio_sessions WHERE performed_on BETWEEN $1::date AND $2::date AND ($3::bigint=0 OR exercise_id=$3) GROUP BY 1
 ) SELECT weeks.start,coalesce(totals.minutes,0) FROM weeks LEFT JOIN totals USING(start) ORDER BY weeks.start`, from, to, exerciseID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var day time.Time
		var minutes float64
		if err = rows.Scan(&day, &minutes); err != nil {
			return a, err
		}
		a.Points = append(a.Points, domain.ChartPoint{Date: day, Label: "Week of " + day.Format("02 Jan 2006"), Value: minutes})
	}
	return a, rows.Err()
}

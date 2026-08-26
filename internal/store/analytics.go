package store

import (
	"context"
	"fmt"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func (s *Store) RoutineAnalytics(ctx context.Context, id int64, fromText, toText string) (domain.RoutineAnalytics, error) {
	from, to, err := parseDateRange(fromText, toText)
	if err != nil {
		return domain.RoutineAnalytics{}, err
	}
	routine, err := s.Routine(ctx, id)
	if err != nil {
		return domain.RoutineAnalytics{}, err
	}
	a := domain.RoutineAnalytics{Routine: routine, From: from, To: to}
	err = s.pool.QueryRow(ctx, `SELECT count(DISTINCT s.id),count(ss.id) FILTER (WHERE NOT ss.skipped),coalesce(sum(ss.reps) FILTER (WHERE NOT ss.skipped),0) FROM sessions s LEFT JOIN session_exercises se ON se.session_id=s.id LEFT JOIN session_sets ss ON ss.session_exercise_id=se.id WHERE s.routine_id=$1 AND s.status='completed' AND s.performed_on BETWEEN $2::date AND $3::date`, id, from, to).Scan(&a.Sessions, &a.Sets, &a.Reps)
	if err != nil {
		return a, err
	}
	weeks := to.Sub(from).Hours() / 24 / 7
	if weeks < 1 {
		weeks = 1
	}
	a.SessionsPerWeek = float64(a.Sessions) / weeks
	rows, err := s.pool.Query(ctx, `SELECT performed_on,count(*) FROM sessions WHERE routine_id=$1 AND status='completed' AND performed_on BETWEEN $2::date AND $3::date GROUP BY performed_on ORDER BY performed_on`, id, from, to)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	var previous *time.Time
	for rows.Next() {
		var day time.Time
		var count float64
		if err := rows.Scan(&day, &count); err != nil {
			return a, err
		}
		a.Points = append(a.Points, domain.ChartPoint{Label: day.Format("02 Jan"), Value: count})
		if previous != nil {
			gap := int(day.Sub(*previous).Hours() / 24)
			if gap > a.LongestGapDays {
				a.LongestGapDays = gap
			}
		}
		copy := day
		previous = &copy
	}
	if err = rows.Err(); err != nil {
		return a, err
	}
	rows.Close()
	a.Exercises, err = s.exerciseSummaries(ctx, `s.routine_id=$1 AND`, []any{id}, from, to)
	return a, err
}

func (s *Store) AnalyticsOverview(ctx context.Context, fromText, toText string) (domain.AnalyticsOverview, error) {
	from, to, err := parseDateRange(fromText, toText)
	if err != nil {
		return domain.AnalyticsOverview{}, err
	}
	a := domain.AnalyticsOverview{From: from, To: to}
	a.Exercises, err = s.exerciseSummaries(ctx, "", nil, from, to)
	return a, err
}

func (s *Store) exerciseSummaries(ctx context.Context, prefix string, args []any, from, to time.Time) ([]domain.ExerciseSummary, error) {
	fromIndex := len(args) + 1
	toIndex := fromIndex + 1
	query := fmt.Sprintf(`SELECT e.id,e.name,e.mode,e.archived,count(DISTINCT s.id),count(ss.id),coalesce(sum(ss.reps),0),coalesce(sum(ss.reps*ss.weight_kg) FILTER (WHERE e.mode='weighted'),0) FROM sessions s JOIN session_exercises se ON se.session_id=s.id JOIN session_sets ss ON ss.session_exercise_id=se.id JOIN exercises e ON e.id=se.exercise_id WHERE %s s.status='completed' AND NOT ss.skipped AND s.performed_on BETWEEN $%d::date AND $%d::date GROUP BY e.id,e.name,e.mode,e.archived ORDER BY e.name`, prefix, fromIndex, toIndex)
	args = append(args, from, to)
	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExerciseSummary
	for rows.Next() {
		var x domain.ExerciseSummary
		if err := rows.Scan(&x.Exercise.ID, &x.Exercise.Name, &x.Exercise.Mode, &x.Exercise.Archived, &x.Sessions, &x.Sets, &x.TotalReps, &x.Volume); err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}

func (s *Store) ExerciseAnalytics(ctx context.Context, id int64, fromText, toText string) (domain.ExerciseAnalytics, error) {
	from, to, err := parseDateRange(fromText, toText)
	if err != nil {
		return domain.ExerciseAnalytics{}, err
	}
	exercise, err := s.Exercise(ctx, id)
	if err != nil {
		return domain.ExerciseAnalytics{}, err
	}
	a := domain.ExerciseAnalytics{Exercise: exercise, From: from, To: to}
	err = s.pool.QueryRow(ctx, `SELECT count(DISTINCT s.id) FILTER (WHERE NOT ss.skipped),coalesce(sum(ss.reps) FILTER (WHERE NOT ss.skipped),0),coalesce(sum(ss.reps*ss.weight_kg) FILTER (WHERE NOT ss.skipped AND se.tracking_mode='weighted'),0),coalesce(max(ss.weight_kg) FILTER (WHERE NOT ss.skipped),0),coalesce(max(ss.reps) FILTER (WHERE NOT ss.skipped),0),coalesce(max(ss.weight_kg*(1+ss.reps/30.0)) FILTER (WHERE NOT ss.skipped AND se.tracking_mode='weighted'),0) FROM sessions s JOIN session_exercises se ON se.session_id=s.id JOIN session_sets ss ON ss.session_exercise_id=se.id WHERE se.exercise_id=$1 AND s.status='completed' AND s.performed_on BETWEEN $2::date AND $3::date`, id, from, to).Scan(&a.Sessions, &a.TotalReps, &a.Volume, &a.BestWeight, &a.BestSetReps, &a.Estimated1RM)
	if err != nil {
		return a, err
	}
	metric := "sum(ss.reps)"
	if exercise.Mode == "weighted" {
		metric = "sum(ss.reps*ss.weight_kg)"
	}
	query := fmt.Sprintf(`SELECT s.performed_on,%s FROM sessions s JOIN session_exercises se ON se.session_id=s.id JOIN session_sets ss ON ss.session_exercise_id=se.id WHERE se.exercise_id=$1 AND s.status='completed' AND NOT ss.skipped AND s.performed_on BETWEEN $2::date AND $3::date GROUP BY s.performed_on ORDER BY s.performed_on`, metric)
	rows, err := s.pool.Query(ctx, query, id, from, to)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var day time.Time
		var value float64
		if err := rows.Scan(&day, &value); err != nil {
			return a, err
		}
		a.Points = append(a.Points, domain.ChartPoint{Label: day.Format("02 Jan"), Value: value})
	}
	return a, rows.Err()
}

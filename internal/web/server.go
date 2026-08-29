package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/local/fitness-tracker/internal/domain"
	"github.com/local/fitness-tracker/internal/store"
)

type Server struct{ db *store.Store }

func New(db *store.Store) http.Handler {
	s := &Server{db: db}
	mux := http.NewServeMux()
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	mux.HandleFunc("GET /", s.dashboard)
	mux.HandleFunc("GET /exercises", s.exercises)
	mux.HandleFunc("POST /exercises", s.createExercise)
	mux.HandleFunc("POST /exercises/{id}/archive", s.archiveExercise)
	mux.HandleFunc("GET /routines", s.routines)
	mux.HandleFunc("POST /routines", s.createRoutine)
	mux.HandleFunc("GET /routines/{id}", s.routine)
	mux.HandleFunc("POST /routines/{id}/activate", s.activateRoutine)
	mux.HandleFunc("POST /routines/{id}/archive", s.archiveRoutine)
	mux.HandleFunc("POST /routines/{id}/duplicate", s.duplicateRoutine)
	mux.HandleFunc("POST /routines/{id}/workouts", s.createWorkout)
	mux.HandleFunc("GET /workouts/{id}", s.workout)
	mux.HandleFunc("POST /workouts/{id}/movements", s.addMovement)
	mux.HandleFunc("POST /workouts/{id}/movements/{index}/remove", s.removeMovement)
	mux.HandleFunc("POST /workouts/{id}/move", s.moveWorkout)
	mux.HandleFunc("POST /workouts/{id}/delete", s.deleteWorkout)
	mux.HandleFunc("POST /workouts/{id}/start", s.startSession)
	mux.HandleFunc("GET /sessions", s.sessions)
	mux.HandleFunc("POST /sessions", s.createSession)
	mux.HandleFunc("GET /sessions/{id}", s.session)
	mux.HandleFunc("POST /sessions/{id}/save", s.saveSession)
	mux.HandleFunc("POST /sessions/{id}/update", s.updateSession)
	mux.HandleFunc("POST /sessions/{id}/sets", s.addSet)
	mux.HandleFunc("POST /sessions/{id}/sets/{setID}/delete", s.deleteSet)
	mux.HandleFunc("POST /sessions/{id}/complete", s.completeSession)
	mux.HandleFunc("POST /sessions/{id}/discard", s.discardSession)
	mux.HandleFunc("POST /sessions/{id}/delete", s.deleteSession)
	mux.HandleFunc("GET /analytics", s.analytics)
	mux.HandleFunc("GET /analytics/routines/{id}", s.routineAnalytics)
	mux.HandleFunc("GET /analytics/exercises/{id}", s.exerciseAnalytics)
	mux.HandleFunc("GET /about", func(w http.ResponseWriter, r *http.Request) { s.render(w, r, AboutPage()) })
	return recoverMiddleware(logMiddleware(originMiddleware(mux)))
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := c.Render(r.Context(), w); err != nil {
		log.Printf("render: %v", err)
	}
}
func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusBadRequest
	if errors.Is(err, store.ErrNotFound) {
		status = http.StatusNotFound
	}
	w.WriteHeader(status)
	s.render(w, r, ErrorPage(status, err.Error()))
}
func idParam(r *http.Request, name string) (int64, error) {
	return strconv.ParseInt(r.PathValue(name), 10, 64)
}
func redirect(w http.ResponseWriter, r *http.Request, target string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", target)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	d, err := s.db.Dashboard(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, DashboardPage(d))
}

func (s *Server) exercises(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.Exercises(r.Context(), r.URL.Query().Get("archived") == "1")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, ExercisesPage(items, r.URL.Query().Get("archived") == "1"))
}
func (s *Server) createExercise(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.db.CreateExercise(r.Context(), r.FormValue("name"), r.FormValue("mode")); err != nil {
		s.fail(w, r, fmt.Errorf("create exercise: %w", err))
		return
	}
	redirect(w, r, "/exercises")
}
func (s *Server) archiveExercise(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.ArchiveExercise(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/exercises")
}

func (s *Server) routines(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.Routines(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, RoutinesPage(items))
}
func (s *Server) createRoutine(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	id, err := s.db.CreateRoutine(r.Context(), r.FormValue("name"), r.FormValue("description"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/routines/%d", id))
}
func (s *Server) routine(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	routine, err := s.db.Routine(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	workouts, err := s.db.Workouts(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, RoutinePage(routine, workouts))
}
func (s *Server) activateRoutine(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.ActivateRoutine(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/routines/%d", id))
}
func (s *Server) archiveRoutine(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.ArchiveRoutine(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/routines")
}
func (s *Server) duplicateRoutine(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	newID, err := s.db.DuplicateRoutine(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/routines/%d", newID))
}

func (s *Server) createWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	if strings.TrimSpace(r.FormValue("name")) == "" {
		s.fail(w, r, errors.New("workout name is required"))
		return
	}
	workoutID, err := s.db.CreateWorkout(r.Context(), id, r.FormValue("name"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/workouts/%d", workoutID))
}
func (s *Server) workout(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	workout, err := s.db.Workout(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	routine, err := s.db.Routine(r.Context(), workout.RoutineID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	exercises, err := s.db.Exercises(r.Context(), false)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, WorkoutPage(routine, workout, exercises))
}
func optionalInt(text string) (int, error) {
	if strings.TrimSpace(text) == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(text)
	if err != nil || value < 0 {
		return 0, errors.New("numeric values cannot be negative")
	}
	return value, nil
}
func (s *Server) addMovement(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	exerciseID, _ := strconv.ParseInt(r.FormValue("exercise_id"), 10, 64)
	format := r.FormValue("format")
	setCount, err := optionalInt(r.FormValue("sets"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	targetReps, err := optionalInt(r.FormValue("target_reps"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	weight, _ := strconv.ParseFloat(r.FormValue("weight"), 64)
	startMinute, _ := strconv.Atoi(r.FormValue("start_minute"))
	duration, _ := strconv.Atoi(r.FormValue("duration"))
	interval, _ := strconv.Atoi(r.FormValue("interval_minutes"))
	if format == "sets_reps" {
		duration = 0
		interval = 0
		startMinute = 0
	}
	if err = s.db.AddMovement(r.Context(), id, exerciseID, format, setCount, targetReps, weight, startMinute, duration, interval); err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/workouts/%d", id))
}
func (s *Server) removeMovement(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	index64, e := idParam(r, "index")
	if err == nil {
		err = e
	}
	if err == nil {
		err = s.db.RemoveMovement(r.Context(), id, int(index64))
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/workouts/%d", id))
}
func (s *Server) moveWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	_ = r.ParseForm()
	direction := 1
	if r.FormValue("direction") == "up" {
		direction = -1
	}
	workout, err := s.db.Workout(r.Context(), id)
	if err == nil {
		err = s.db.MoveWorkout(r.Context(), id, direction)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/routines/%d", workout.RoutineID))
}
func (s *Server) deleteWorkout(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	workout, err := s.db.Workout(r.Context(), id)
	if err == nil {
		err = s.db.DeleteWorkout(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/routines/%d", workout.RoutineID))
}

func (s *Server) startSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions?new=1&workout_id=%d", id))
}
func (s *Server) sessions(w http.ResponseWriter, r *http.Request) {
	items, err := s.db.Sessions(r.Context(), 100)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	selectedWorkoutID, _ := strconv.ParseInt(r.URL.Query().Get("workout_id"), 10, 64)
	showNewForm := r.URL.Query().Get("new") == "1"
	var groups []domain.WorkoutGroup
	if showNewForm {
		groups, err = s.db.WorkoutGroups(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	s.render(w, r, SessionsPage(items, groups, showNewForm, selectedWorkoutID, time.Now()))
}
func parsePerformedOn(text string) (time.Time, error) {
	if strings.TrimSpace(text) == "" {
		return time.Time{}, errors.New("performed date is required")
	}
	value, err := time.Parse("2006-01-02", text)
	if err != nil {
		return time.Time{}, errors.New("performed date must be a valid date")
	}
	return value, nil
}
func (s *Server) createSession(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	workoutID, err := strconv.ParseInt(r.FormValue("workout_id"), 10, 64)
	if err != nil || workoutID <= 0 {
		s.fail(w, r, errors.New("choose a workout"))
		return
	}
	performedOn, err := parsePerformedOn(r.FormValue("performed_on"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sessionID, err := s.db.StartSession(r.Context(), workoutID, performedOn)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions/%d", sessionID))
}
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	session, exercises, err := s.db.Session(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, SessionPage(session, exercises, r.URL.Query().Get("edit") == "1"))
}
func parseSetUpdates(r *http.Request) []store.SetUpdate {
	var out []store.SetUpdate
	for key, values := range r.PostForm {
		if !strings.HasPrefix(key, "reps_") || len(values) == 0 {
			continue
		}
		idText := strings.TrimPrefix(key, "reps_")
		id, _ := strconv.ParseInt(idText, 10, 64)
		reps, _ := strconv.Atoi(values[0])
		weight, _ := strconv.ParseFloat(r.FormValue("weight_"+idText), 64)
		skipped := r.FormValue("skipped_"+idText) == "on"
		out = append(out, store.SetUpdate{ID: id, Reps: reps, WeightKG: weight, Skipped: skipped})
	}
	return out
}
func (s *Server) saveSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	performedOn, err := parsePerformedOn(r.FormValue("performed_on"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rpe *int
	if text := r.FormValue("rpe"); text != "" {
		v, e := strconv.Atoi(text)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		rpe = &v
	}
	if err = s.db.SaveDraft(r.Context(), id, performedOn, r.FormValue("notes"), rpe, parseSetUpdates(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, SaveStatus())
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions/%d", id))
}

func (s *Server) updateSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = r.ParseForm()
	}
	performedOn := time.Time{}
	if err == nil {
		performedOn, err = parsePerformedOn(r.FormValue("performed_on"))
	}
	var rpe *int
	if err == nil {
		if text := r.FormValue("rpe"); text != "" {
			var value int
			value, err = strconv.Atoi(text)
			if err == nil {
				rpe = &value
			}
		}
	}
	if err == nil {
		err = s.db.UpdateCompletedSession(r.Context(), id, performedOn, r.FormValue("notes"), rpe, parseSetUpdates(r))
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions/%d", id))
}
func (s *Server) addSet(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	performedOn, err := parsePerformedOn(r.FormValue("performed_on"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rpe *int
	if text := r.FormValue("rpe"); text != "" {
		v, e := strconv.Atoi(text)
		if e != nil {
			s.fail(w, r, e)
			return
		}
		rpe = &v
	}
	if err = s.db.SaveDraft(r.Context(), id, performedOn, r.FormValue("notes"), rpe, parseSetUpdates(r)); err != nil {
		s.fail(w, r, err)
		return
	}
	exerciseID, _ := strconv.ParseInt(r.FormValue("session_exercise_id"), 10, 64)
	if err = s.db.AddSessionSet(r.Context(), id, exerciseID); err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		s.renderSessionExerciseUpdate(w, r, id, exerciseID, true, true)
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions/%d#exercise-%d", id, exerciseID))
}

func (s *Server) deleteSet(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	setID, err := idParam(r, "setID")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	session, _, err := s.db.Session(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	performedOn, err := parsePerformedOn(r.FormValue("performed_on"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var rpe *int
	if text := r.FormValue("rpe"); text != "" {
		value, parseErr := strconv.Atoi(text)
		if parseErr != nil {
			s.fail(w, r, parseErr)
			return
		}
		rpe = &value
	}
	updates := parseSetUpdates(r)
	if session.Status == "draft" {
		err = s.db.SaveDraft(r.Context(), id, performedOn, r.FormValue("notes"), rpe, updates)
	} else if session.Status == "completed" {
		err = s.db.UpdateCompletedSession(r.Context(), id, performedOn, r.FormValue("notes"), rpe, updates)
	} else {
		err = store.ErrNotFound
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	exerciseID, err := s.db.DeleteSessionSet(r.Context(), id, setID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") == "true" {
		s.renderSessionExerciseUpdate(w, r, id, exerciseID, true, session.Status == "draft")
		return
	}
	target := fmt.Sprintf("/sessions/%d#exercise-%d", id, exerciseID)
	if session.Status == "completed" {
		target = fmt.Sprintf("/sessions/%d?edit=1#exercise-%d", id, exerciseID)
	}
	redirect(w, r, target)
}

func (s *Server) renderSessionExerciseUpdate(w http.ResponseWriter, r *http.Request, sessionID, exerciseID int64, editable, addable bool) {
	session, exercises, err := s.db.Session(r.Context(), sessionID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	for _, exercise := range exercises {
		if exercise.ID == exerciseID {
			s.render(w, r, SessionExerciseUpdate(exercises, editable, addable, session.CompletedSets))
			return
		}
	}
	s.fail(w, r, store.ErrNotFound)
}
func (s *Server) completeSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = r.ParseForm()
	}
	if err == nil {
		var performedOn time.Time
		performedOn, err = parsePerformedOn(r.FormValue("performed_on"))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		var rpe *int
		if text := r.FormValue("rpe"); text != "" {
			v, _ := strconv.Atoi(text)
			rpe = &v
		}
		err = s.db.SaveDraft(r.Context(), id, performedOn, r.FormValue("notes"), rpe, parseSetUpdates(r))
	}
	if err == nil {
		err = s.db.CompleteSession(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, fmt.Sprintf("/sessions/%d", id))
}
func (s *Server) discardSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.DiscardSession(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/")
}
func (s *Server) deleteSession(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.DeleteSession(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/sessions")
}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	a, err := s.db.AnalyticsOverview(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, AnalyticsPage(a))
}
func (s *Server) routineAnalytics(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.db.RoutineAnalytics(r.Context(), id, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, RoutineAnalyticsPage(a))
}
func (s *Server) exerciseAnalytics(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	a, err := s.db.ExerciseAnalytics(r.Context(), id, r.URL.Query().Get("from"), r.URL.Query().Get("to"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, ExerciseAnalyticsPage(a))
}

func originMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if raw := r.Header.Get("Origin"); raw != "" {
				origin, err := url.Parse(raw)
				if err != nil || !strings.EqualFold(origin.Host, r.Host) {
					http.Error(w, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				log.Printf("panic: %v", value)
				http.Error(w, "internal server error", 500)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

var _ context.Context
var _ = domain.ChartPoint{}

package web

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

type cardioForm struct {
	ID                                             int64
	ExerciseID, PerformedOn, Minutes, Notes, Error string
}

func cardioOptions(items []domain.Exercise, selected string) []domain.Exercise {
	var out []domain.Exercise
	for _, e := range items {
		if e.Mode == "cardio" && (!e.Archived || strconv.FormatInt(e.ID, 10) == selected) {
			out = append(out, e)
		}
	}
	return out
}

func (s *Server) renderCardioForm(w http.ResponseWriter, r *http.Request, f cardioForm) {
	items, err := s.db.Exercises(r.Context(), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if f.Error != "" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusBadRequest)
	}
	s.render(w, r, CardioFormPage(f, cardioOptions(items, f.ExerciseID)))
}

func (s *Server) newCardio(w http.ResponseWriter, r *http.Request) {
	s.renderCardioForm(w, r, cardioForm{PerformedOn: inputDate(time.Now())})
}

func (s *Server) cardio(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c, err := s.db.CardioSession(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.URL.Query().Get("edit") == "1" {
		s.renderCardioForm(w, r, cardioForm{ID: c.ID, ExerciseID: strconv.FormatInt(c.ExerciseID, 10), PerformedOn: inputDate(c.PerformedOn), Minutes: number(c.DurationMinutes), Notes: c.Notes})
		return
	}
	s.render(w, r, CardioPage(c))
}

func (s *Server) saveCardio(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	f := cardioForm{ExerciseID: r.FormValue("exercise_id"), PerformedOn: r.FormValue("performed_on"), Minutes: r.FormValue("minutes"), Notes: r.FormValue("notes")}
	if r.PathValue("id") != "" {
		id, err := idParam(r, "id")
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if _, err = s.db.CardioSession(r.Context(), id); err != nil {
			s.fail(w, r, err)
			return
		}
		f.ID = id
	}
	c, err := parseCardioForm(f)
	if err == nil {
		c.ID, err = s.db.SaveCardio(r.Context(), c)
	}
	if err != nil {
		f.Error = err.Error()
		s.renderCardioForm(w, r, f)
		return
	}
	redirect(w, r, fmt.Sprintf("/cardio/%d", c.ID))
}

func parseCardioForm(f cardioForm) (domain.CardioSession, error) {
	c := domain.CardioSession{ID: f.ID, Notes: f.Notes}
	var err error
	c.ExerciseID, _ = strconv.ParseInt(f.ExerciseID, 10, 64)
	c.PerformedOn, err = parsePerformedOn(f.PerformedOn)
	if err != nil {
		return c, err
	}
	c.DurationMinutes, err = strconv.ParseFloat(f.Minutes, 64)
	if err != nil {
		return c, fmt.Errorf("enter a valid duration in minutes")
	}
	return c, domain.ValidateCardio(c)
}

func (s *Server) deleteCardio(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err == nil {
		err = s.db.DeleteCardio(r.Context(), id)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	redirect(w, r, "/sessions")
}

func (s *Server) cardioAnalytics(w http.ResponseWriter, r *http.Request) {
	var exerciseID int64
	if text := r.URL.Query().Get("exercise_id"); text != "" {
		var err error
		exerciseID, err = strconv.ParseInt(text, 10, 64)
		if err != nil || exerciseID < 0 {
			s.fail(w, r, fmt.Errorf("choose a cardio activity"))
			return
		}
	}
	a, err := s.db.CardioAnalytics(r.Context(), r.URL.Query().Get("from"), r.URL.Query().Get("to"), exerciseID)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, err := s.db.Exercises(r.Context(), true)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, CardioAnalyticsPage(a, items))
}

func exerciseAnalyticsURL(e domain.Exercise) string {
	if e.Mode == "cardio" {
		return fmt.Sprintf("/analytics/cardio?exercise_id=%d", e.ID)
	}
	return fmt.Sprintf("/analytics/exercises/%d", e.ID)
}

func trainingMonthStarts(items []domain.TrainingEntry, i int) bool {
	return i == 0 || sessionMonth(items[i].PerformedOn) != sessionMonth(items[i-1].PerformedOn)
}

func cardioFormAction(id int64) string {
	if id == 0 {
		return "/cardio"
	}
	return fmt.Sprintf("/cardio/%d/update", id)
}

func cardioFormCancel(id int64) string {
	if id == 0 {
		return "/sessions"
	}
	return fmt.Sprintf("/cardio/%d", id)
}

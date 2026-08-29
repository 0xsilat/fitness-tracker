package web

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/local/fitness-tracker/internal/domain"
	"github.com/local/fitness-tracker/internal/store"
)

type emomEntryState struct {
	Reps, Weight, Message        string
	Open, Confirm, PreserveQuick bool
}

func emomStateFor(e domain.SessionExercise, states map[int64]emomEntryState) emomEntryState {
	if state, ok := states[e.ID]; ok {
		return state
	}
	return emomState(e, nil)
}

func emomState(e domain.SessionExercise, form map[string][]string) emomEntryState {
	state := emomEntryState{}
	if len(e.Sets) > 0 {
		first := e.Sets[0]
		state.Reps = actualRepsValue(first.Reps)
		for _, set := range e.Sets {
			if set.Reps != first.Reps {
				state.Reps = ""
			}
			if set.Skipped || set.Reps != first.Reps || (e.Mode == "weighted" && set.WeightKG != first.WeightKG) {
				state.Open = true
			}
		}
	}
	key := strconv.FormatInt(e.ID, 10)
	if _, ok := form["emom_present_"+key]; ok {
		state.Open = len(form["emom_open_"+key]) > 0
	}
	if values, ok := form["emom_reps_"+key]; ok && len(values) > 0 {
		state.Reps = values[0]
	}
	if values, ok := form["emom_weight_"+key]; ok && len(values) > 0 {
		state.Weight = values[0]
	}
	return state
}

func emomSummary(e domain.SessionExercise) string {
	done, skipped := 0, 0
	var first domain.SessionSet
	repsVary, weightsVary := false, false
	for _, set := range e.Sets {
		if set.Skipped {
			skipped++
			continue
		}
		if set.Reps <= 0 {
			continue
		}
		if done == 0 {
			first = set
		}
		repsVary = repsVary || set.Reps != first.Reps
		weightsVary = weightsVary || set.WeightKG != first.WeightKG
		done++
	}
	parts := []string{fmt.Sprintf("%d of %d minutes logged", done, len(e.Sets))}
	if done > 0 {
		if repsVary {
			parts = append(parts, "Reps vary")
		} else {
			parts = append(parts, fmt.Sprintf("%d reps each", first.Reps))
		}
		if e.Mode == "weighted" {
			if weightsVary {
				parts = append(parts, "Weights vary")
			} else {
				parts = append(parts, number(first.WeightKG)+" kg each")
			}
		}
	}
	if incomplete := len(e.Sets) - done - skipped; incomplete > 0 {
		parts = append(parts, fmt.Sprintf("%d incomplete", incomplete))
	}
	if skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", skipped))
	}
	return strings.Join(parts, " · ")
}

// Overlay only known sets from this session. Never trust submitted set IDs to
// select records, and never turn missing/invalid inputs into zero-valued work.
func overlaySessionForm(r *http.Request, session *domain.Session, exercises []domain.SessionExercise) ([]store.SetUpdate, error) {
	performedOn, err := parsePerformedOn(r.PostForm.Get("performed_on"))
	if err != nil {
		return nil, err
	}
	var rpe *int
	if text := r.PostForm.Get("rpe"); text != "" {
		value, err := strconv.Atoi(text)
		if err != nil || value < 1 || value > 10 {
			return nil, errors.New("effort must be between 1 and 10")
		}
		rpe = &value
	}
	var updates []store.SetUpdate
	for i := range exercises {
		for j := range exercises[i].Sets {
			set := &exercises[i].Sets[j]
			key := strconv.FormatInt(set.ID, 10)
			if !r.PostForm.Has("reps_" + key) {
				return nil, errors.New("the session changed; reload before applying shared reps")
			}
			reps, err := optionalInt(r.PostForm.Get("reps_" + key))
			if err != nil {
				return nil, errors.New("actual reps must be a nonnegative whole number")
			}
			weight := set.WeightKG
			if exercises[i].Mode == "weighted" {
				text := r.PostForm.Get("weight_" + key)
				weight = 0
				if text != "" {
					weight, err = strconv.ParseFloat(text, 64)
				}
				if err != nil || math.IsNaN(weight) || math.IsInf(weight, 0) || weight < 0 {
					return nil, errors.New("weight must be a nonnegative number")
				}
			}
			set.Reps, set.WeightKG, set.Skipped = reps, weight, r.PostForm.Get("skipped_"+key) == "on"
			updates = append(updates, store.SetUpdate{ID: set.ID, Reps: reps, WeightKG: weight, Skipped: set.Skipped})
		}
	}
	session.PerformedOn, session.Notes, session.RPE = performedOn, r.PostForm.Get("notes"), rpe
	return updates, nil
}

func applyEMOM(e *domain.SessionExercise, state *emomEntryState, confirmed bool) error {
	reps, err := strconv.Atoi(strings.TrimSpace(state.Reps))
	if err != nil || reps <= 0 {
		return errors.New("enter a positive whole number of reps per minute")
	}
	var weight *float64
	if e.Mode == "weighted" && strings.TrimSpace(state.Weight) != "" {
		value, err := strconv.ParseFloat(strings.TrimSpace(state.Weight), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || math.Mod(value*4, 1) != 0 {
			return errors.New("weight must be nonnegative in 0.25 kg increments")
		}
		weight = &value
	}
	count, replacing := 0, false
	for _, set := range e.Sets {
		if set.Skipped {
			continue
		}
		count++
		replacing = replacing || (set.Reps > 0 && set.Reps != reps) || (weight != nil && set.WeightKG != *weight)
	}
	if count == 0 {
		return errors.New("all minutes are skipped; unskip a minute before applying reps")
	}
	if replacing && !confirmed {
		state.Confirm = true
		return nil
	}
	for i := range e.Sets {
		if e.Sets[i].Skipped {
			continue
		}
		e.Sets[i].Reps = reps
		if weight != nil {
			e.Sets[i].WeightKG = *weight
		}
	}
	state.Message = fmt.Sprintf("Applied to %d minutes.", count)
	return nil
}

func (s *Server) emomEntry(w http.ResponseWriter, r *http.Request) {
	id, err := idParam(r, "id")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	exerciseID, err := idParam(r, "exerciseID")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err = r.ParseForm(); err != nil {
		s.fail(w, r, err)
		return
	}
	session, exercises, err := s.db.Session(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	index := -1
	for i, e := range exercises {
		if e.ID == exerciseID && e.Format == "emom" {
			index = i
			break
		}
	}
	if index < 0 {
		s.fail(w, r, store.ErrNotFound)
		return
	}
	_, err = overlaySessionForm(r, &session, exercises)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	e := &exercises[index]
	state := emomState(*e, r.PostForm)
	action := r.PostForm.Get("emom_action")
	if action == "summary" {
		s.render(w, r, EMOMSummary(*e, false))
		return
	}
	saved := false
	switch action {
	case "cancel":
		state.Message = "Existing minute values kept."
	case "apply", "confirm", "":
		if err = applyEMOM(e, &state, action == "confirm"); err != nil {
			state.Message = err.Error()
		} else if !state.Confirm && session.Status == "draft" {
			var updates []store.SetUpdate
			for _, exercise := range exercises {
				for _, set := range exercise.Sets {
					updates = append(updates, store.SetUpdate{ID: set.ID, Reps: set.Reps, WeightKG: set.WeightKG, Skipped: set.Skipped})
				}
			}
			if err = s.db.SaveDraft(r.Context(), id, session.PerformedOn, session.Notes, session.RPE, updates); err != nil {
				s.fail(w, r, err)
				return
			}
			saved = true
		}
	default:
		s.fail(w, r, errors.New("invalid EMOM action"))
		return
	}
	session.CompletedSets = 0
	for _, exercise := range exercises {
		session.CompletedSets += completedExerciseSets(exercise)
	}
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, SessionExerciseUpdate(*e, state, session.Status == "draft", session.CompletedSets, saved))
		return
	}
	s.render(w, r, SessionPageWithState(session, exercises, true, map[int64]emomEntryState{exerciseID: state}))
}

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
	Reps, Weight, Count, Message string
	Open, Confirm, PreserveQuick bool
	Form                         map[string][]string // Raw values are retained only when session validation fails.
}

func rawValue(form map[string][]string, key, fallback string) string {
	if values, ok := form[key]; ok && len(values) > 0 {
		return values[0]
	}
	return fallback
}

func sessionValues(states map[int64]emomEntryState) map[string][]string {
	for _, state := range states {
		if state.Form != nil {
			return state.Form
		}
	}
	return nil
}

func emomStateFor(e domain.SessionExercise, states map[int64]emomEntryState) emomEntryState {
	if state, ok := states[e.ID]; ok {
		return state
	}
	return emomState(e, nil)
}

func emomState(e domain.SessionExercise, form map[string][]string) emomEntryState {
	state := emomEntryState{}
	if done := completedExerciseSets(e); done > 0 && done != defaultEMOMCount(e) {
		state.Count = strconv.Itoa(done)
	}
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
	if values, ok := form["emom_count_"+key]; ok && len(values) > 0 {
		state.Count = values[0]
	}
	return state
}

// Limit bulk allocations and form size; explicit invalid values never use defaults.
const maxEMOMMinutes = 10000

func defaultEMOMCount(e domain.SessionExercise) int {
	if e.PlannedMinutes == nil {
		count := 0
		for _, set := range e.Sets {
			if !set.Skipped && !set.Deleted {
				count++
			}
		}
		return count
	}
	count := len(e.PlannedMinutes)
	for _, minute := range e.PlannedMinutes {
		for _, set := range e.Sets {
			if set.Minute == minute && set.Skipped {
				count--
				break
			}
		}
	}
	return count
}

func emomStats(e domain.SessionExercise) string {
	done, reps := 0, 0.0
	for _, set := range e.Sets {
		if !set.Skipped && !set.Deleted && set.Reps > 0 {
			done++
			reps += float64(set.Reps)
		}
	}
	average := "—"
	if done > 0 {
		average = fmt.Sprintf("%.1f", reps/float64(done))
	}
	return fmt.Sprintf("%d minutes done · %s avg reps/min", done, average)
}

func sessionSetKey(set domain.SessionSet) string {
	if set.ID != 0 {
		return strconv.FormatInt(set.ID, 10)
	}
	return fmt.Sprintf("new_%d_%d", set.SessionExerciseID, set.Position)
}

func pendingEMOMCount(e domain.SessionExercise) int {
	count := 0
	for _, set := range e.Sets {
		if set.ID == 0 {
			count++
		}
	}
	return count
}

func appendEMOMMinute(e *domain.SessionExercise) {
	position, minute, weight := 0, 0, 0.0
	interval := max(1, e.IntervalMinutes)
	for _, set := range e.Sets {
		position = max(position, set.Position)
		if set.Minute > 0 {
			minute = max(minute, set.Minute)
		} else {
			minute += interval
		}
		if !set.Skipped && !set.Deleted {
			weight = set.WeightKG
		}
	}
	position = max(position, len(e.Sets)) + 1
	if minute == 0 {
		minute = 1 - interval
	}
	e.Sets = append(e.Sets, domain.SessionSet{SessionExerciseID: e.ID, Position: position, Minute: minute + interval, WeightKG: weight})
}

func sessionUpdates(exercises []domain.SessionExercise) []store.SetUpdate {
	var updates []store.SetUpdate
	for _, e := range exercises {
		for _, set := range e.Sets {
			updates = append(updates, store.SetUpdate{ID: set.ID, Reps: set.Reps, WeightKG: set.WeightKG, Skipped: set.Skipped, SessionExerciseID: e.ID, Position: set.Position, Minute: set.Minute, Deleted: set.Deleted})
		}
	}
	return updates
}

func emomSummary(e domain.SessionExercise) string {
	done, skipped, total := 0, 0, 0
	var first domain.SessionSet
	repsVary, weightsVary := false, false
	for _, set := range e.Sets {
		if set.Deleted {
			continue
		}
		total++
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
	parts := []string{fmt.Sprintf("%d of %d minutes logged", done, total)}
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
	if incomplete := total - done - skipped; incomplete > 0 {
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
	for i := range exercises {
		if text := r.PostForm.Get(fmt.Sprintf("emom_pending_%d", exercises[i].ID)); text != "" {
			count, err := strconv.Atoi(text)
			if err != nil || count < 0 || count > maxEMOMMinutes || exercises[i].Format != "emom" {
				return nil, errors.New("invalid pending EMOM minutes")
			}
			for n := 0; n < count; n++ {
				appendEMOMMinute(&exercises[i])
			}
		}
		for j := range exercises[i].Sets {
			set := &exercises[i].Sets[j]
			set.Deleted = exercises[i].Format == "emom" && set.ID > 0 && r.PostForm.Get("emom_deleted_"+sessionSetKey(*set)) == "1"
		}
	}
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
			key := sessionSetKey(*set)
			if r.PostForm.Get("emom_deleted_"+key) == "1" && exercises[i].Format == "emom" && set.ID > 0 {
				set.Deleted = true
				updates = append(updates, store.SetUpdate{ID: set.ID, SessionExerciseID: exercises[i].ID, Deleted: true})
				continue
			}
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
			updates = append(updates, store.SetUpdate{ID: set.ID, Reps: reps, WeightKG: weight, Skipped: set.Skipped, SessionExerciseID: exercises[i].ID, Position: set.Position, Minute: set.Minute})
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
	count := defaultEMOMCount(*e)
	if text := strings.TrimSpace(state.Count); text != "" {
		count, err = strconv.Atoi(text)
		if err != nil || count <= 0 || count > maxEMOMMinutes {
			return fmt.Errorf("enter a whole number of minutes between 1 and %d", maxEMOMMinutes)
		}
	}
	if count <= 0 {
		return errors.New("all planned minutes are skipped; enter minutes done or unskip a minute")
	}
	n, replacing := 0, false
	for _, set := range e.Sets {
		if set.Skipped || set.Deleted {
			continue
		}
		n++
		if n <= count {
			replacing = replacing || (set.Reps > 0 && set.Reps != reps) || (weight != nil && set.WeightKG != *weight)
		} else {
			replacing = replacing || set.Reps > 0
		}
	}
	if replacing && !confirmed {
		state.Confirm = true
		return nil
	}
	for ; n < count; n++ {
		appendEMOMMinute(e)
	}
	n = 0
	for i := range e.Sets {
		if e.Sets[i].Skipped || e.Sets[i].Deleted {
			continue
		}
		n++
		if n > count {
			e.Sets[i].Reps = 0
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
		states := map[int64]emomEntryState{}
		for _, exercise := range exercises {
			state := emomState(exercise, r.PostForm)
			state.Form = r.PostForm
			if exercise.ID == exerciseID {
				state.Message = err.Error()
				state.Open = true
			}
			states[exercise.ID] = state
		}
		// Replace the entire form so invalid metadata and other exercise inputs survive.
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("HX-Retarget", "body")
			w.Header().Set("HX-Reswap", "innerHTML")
		}
		s.render(w, r, SessionPageWithState(session, exercises, true, states))
		return
	}
	e := &exercises[index]
	state := emomState(*e, r.PostForm)
	action := r.PostForm.Get("emom_action")
	if action == "summary" {
		s.render(w, r, EMOMSummary(*e, false))
		s.render(w, r, EMOMStats(*e, true))
		count := 0
		for _, exercise := range exercises {
			count += completedExerciseSets(exercise)
		}
		s.render(w, r, SessionSetCount(count, true))
		return
	}
	saved := false
	switch {
	case action == "add":
		if session.Status != "draft" {
			state.Message = "Use the minute count to extend this completed session."
			break
		}
		appendEMOMMinute(e)
		state.Open = true
		state.Message = "Minute added."
	case strings.HasPrefix(action, "remove:"):
		key := strings.TrimPrefix(action, "remove:")
		remaining := 0
		for _, set := range e.Sets {
			if !set.Deleted {
				remaining++
			}
		}
		if remaining <= 1 {
			err = errors.New("each exercise must retain at least one minute")
			state.Message = err.Error()
			break
		}
		for i := range e.Sets {
			if sessionSetKey(e.Sets[i]) == key {
				if e.Sets[i].ID == 0 {
					e.Sets = append(e.Sets[:i], e.Sets[i+1:]...)
					for j := i; j < len(e.Sets); j++ {
						if e.Sets[j].ID == 0 {
							e.Sets[j].Position--
							e.Sets[j].Minute -= max(1, e.IntervalMinutes)
						}
					}
				} else {
					e.Sets[i].Deleted = true
				}
				state.Message = "Minute removed."
				break
			}
		}
	case action == "cancel":
		state.Message = "Existing minute values kept."
	case action == "apply" || action == "confirm" || action == "":
		if err = applyEMOM(e, &state, action == "confirm"); err != nil {
			state.Message = err.Error()
		}
	default:
		s.fail(w, r, errors.New("invalid EMOM action"))
		return
	}
	if err == nil && !state.Confirm && action != "cancel" && action != "summary" && session.Status == "draft" {
		if err = s.db.SaveDraft(r.Context(), id, session.PerformedOn, session.Notes, session.RPE, sessionUpdates(exercises)); err != nil {
			s.fail(w, r, err)
			return
		}
		saved = true
		// Replace temporary rows with their persisted IDs before the next request.
		session, exercises, err = s.db.Session(r.Context(), id)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		e = &exercises[index]
	}
	session.CompletedSets = 0
	for _, exercise := range exercises {
		session.CompletedSets += completedExerciseSets(exercise)
	}
	if r.Header.Get("HX-Request") == "true" {
		s.render(w, r, SessionExerciseUpdate(*e, state, session.Status == "draft", session.CompletedSets, saved))
		return
	}
	states := map[int64]emomEntryState{}
	for _, exercise := range exercises {
		states[exercise.ID] = emomState(exercise, r.PostForm)
	}
	states[exerciseID] = state
	s.render(w, r, SessionPageWithState(session, exercises, true, states))
}

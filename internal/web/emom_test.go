package web

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func sampleEMOM() domain.SessionExercise {
	return domain.SessionExercise{ID: 7, SessionID: 42, Format: "emom", Mode: "weighted", Sets: []domain.SessionSet{
		{ID: 11, Minute: 2, Reps: 8, WeightKG: 10},
		{ID: 12, Minute: 5, Reps: 6, WeightKG: 12, Skipped: true},
		{ID: 13, Minute: 8, Reps: 8, WeightKG: 15},
	}}
}

func TestApplyEMOMConfirmationAndPreservedValues(t *testing.T) {
	e := sampleEMOM()
	before := append([]domain.SessionSet(nil), e.Sets...)
	state := emomEntryState{Reps: "10"}
	if err := applyEMOM(&e, &state, false); err != nil {
		t.Fatal(err)
	}
	if !state.Confirm || !reflect.DeepEqual(e.Sets, before) {
		t.Fatal("confirmation must not change existing values")
	}
	state.Confirm = false
	if err := applyEMOM(&e, &state, true); err != nil {
		t.Fatal(err)
	}
	if e.Sets[0].Reps != 10 || e.Sets[2].Reps != 10 || e.Sets[1] != before[1] {
		t.Fatalf("incorrect bulk update: %#v", e.Sets)
	}
	for i := range e.Sets {
		if e.Sets[i].WeightKG != before[i].WeightKG || e.Sets[i].Minute != before[i].Minute {
			t.Fatal("blank weight or scheduled minute was changed")
		}
	}
	state = emomEntryState{Reps: "10", Weight: "0"}
	if err := applyEMOM(&e, &state, true); err != nil {
		t.Fatal(err)
	}
	if e.Sets[0].WeightKG != 0 || e.Sets[2].WeightKG != 0 || e.Sets[1] != before[1] {
		t.Fatal("zero weight must apply, while skips remain unchanged")
	}
}

func TestApplyEMOMRejectsInvalidValuesWithoutMutation(t *testing.T) {
	for _, input := range []emomEntryState{{Reps: ""}, {Reps: "0"}, {Reps: "-1"}, {Reps: "1.5"}, {Reps: "5", Weight: "-1"}, {Reps: "5", Weight: "NaN"}, {Reps: "5", Weight: "+Inf"}, {Reps: "5", Weight: "10.1"}} {
		e := sampleEMOM()
		before := append([]domain.SessionSet(nil), e.Sets...)
		if err := applyEMOM(&e, &input, true); err == nil {
			t.Errorf("accepted %#v", input)
		}
		if !reflect.DeepEqual(e.Sets, before) {
			t.Fatal("invalid input changed entries")
		}
	}
	e := sampleEMOM()
	for i := range e.Sets {
		e.Sets[i].Skipped = true
	}
	if err := applyEMOM(&e, &emomEntryState{Reps: "5"}, true); err == nil {
		t.Fatal("all-skipped exercise should reject bulk entry")
	}
}

func TestEMOMSummaryAndDisclosureState(t *testing.T) {
	e := sampleEMOM()
	if got := emomSummary(e); got != "2 of 3 minutes logged · 8 reps each · Weights vary · 1 skipped" {
		t.Fatal(got)
	}
	if !emomState(e, nil).Open {
		t.Fatal("exceptions should start expanded")
	}
	state := emomState(e, url.Values{"emom_present_7": {"1"}, "emom_reps_7": {"9"}, "emom_weight_7": {"10"}})
	if state.Open || state.Reps != "9" || state.Weight != "10" {
		t.Fatalf("submitted state not preserved: %#v", state)
	}
	for i := range e.Sets {
		e.Sets[i].Reps = 0
		e.Sets[i].WeightKG = 0
		e.Sets[i].Skipped = false
	}
	if emomState(e, nil).Open {
		t.Fatal("empty entries should start collapsed")
	}
	if got := emomSummary(e); got != "0 of 3 minutes logged · 3 incomplete" {
		t.Fatal(got)
	}
}

func TestOverlaySessionFormUsesSubmittedValuesAndKnownSets(t *testing.T) {
	values := url.Values{"performed_on": {"2026-08-30"}, "notes": {"unsaved note"}, "rpe": {"7"}, "reps_11": {"4"}, "weight_11": {"10.25"}, "reps_12": {"6"}, "weight_12": {"12"}, "skipped_12": {"on"}, "reps_13": {"9"}, "weight_13": {"15"}, "reps_999": {"100"}}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	session := domain.Session{}
	exercises := []domain.SessionExercise{sampleEMOM()}
	updates, err := overlaySessionForm(r, &session, exercises)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 3 || updates[0].Reps != 4 || updates[0].WeightKG != 10.25 || !updates[1].Skipped || exercises[0].Sets[2].Reps != 9 {
		t.Fatalf("wrong overlay: %#v", updates)
	}
	if session.Notes != "unsaved note" || *session.RPE != 7 || session.PerformedOn.Format("2006-01-02") != "2026-08-30" {
		t.Fatal("session metadata not preserved")
	}
	r.PostForm.Del("reps_11")
	if _, err := overlaySessionForm(r, &session, exercises); err == nil {
		t.Fatal("missing current form data must not silently use stale DB values")
	}
	r.PostForm.Set("reps_11", "oops")
	if _, err := overlaySessionForm(r, &session, exercises); err == nil {
		t.Fatal("invalid reps must not become zero")
	}
}

func TestEMOMFormsUseHTMXWithoutCustomScripting(t *testing.T) {
	session := domain.Session{ID: 42, Status: "draft", PerformedOn: time.Now()}
	var output bytes.Buffer
	if err := SessionPage(session, []domain.SessionExercise{sampleEMOM()}, false).Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	html := output.String()
	for _, want := range []string{`id="emom-form-7"`, `hx-include="#session-form"`, `hx-sync="#session-form:queue all"`, `hx-disabled-elt="#session-fields"`, `/sessions/42/exercises/7/emom`, `form="emom-form-7"`, `name="emom_present_7"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(html, "hx-on:") || strings.Contains(html, "initializeEMOM") {
		t.Fatal("EMOM must not require custom event scripts")
	}
}

package web

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/local/fitness-tracker/internal/domain"
)

func TestHTMXIntegrityMatchesBundledScript(t *testing.T) {
	script, err := os.ReadFile("../../static/htmx.min.js")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha512.Sum384(script)
	var output bytes.Buffer
	if err := Layout("test").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	want := `integrity="sha384-` + base64.StdEncoding.EncodeToString(digest[:]) + `"`
	if !strings.Contains(output.String(), want) || !strings.Contains(output.String(), `src="/static/htmx.min.js?v=2.0.4"`) {
		t.Fatal("the browser would reject HTMX or need the CDN")
	}
}

func TestEMOMExactCountsAndDefaults(t *testing.T) {
	e := sampleEMOM()
	e.IntervalMinutes = 3
	e.PlannedMinutes = []int{2, 5, 8}
	state := emomEntryState{Reps: "10", Count: "1"}
	before := append([]domain.SessionSet(nil), e.Sets...)
	if err := applyEMOM(&e, &state, false); err != nil || !state.Confirm || !reflect.DeepEqual(before, e.Sets) {
		t.Fatalf("confirmation: %v %#v", err, e)
	}
	if err := applyEMOM(&e, &state, true); err != nil {
		t.Fatal(err)
	}
	if completedExerciseSets(e) != 1 || e.Sets[2].Reps != 0 || e.Sets[1] != before[1] || e.Sets[2].WeightKG != 15 {
		t.Fatalf("short count: %#v", e.Sets)
	}
	state = emomEntryState{Reps: "10", Count: "4"}
	if err := applyEMOM(&e, &state, true); err != nil {
		t.Fatal(err)
	}
	if completedExerciseSets(e) != 4 || len(e.Sets) != 5 || e.Sets[3].Minute != 11 || e.Sets[4].Minute != 14 || e.Sets[4].WeightKG != 15 {
		t.Fatalf("extended: %#v", e.Sets)
	}
	state = emomEntryState{Reps: "10"}
	if err := applyEMOM(&e, &state, true); err != nil {
		t.Fatal(err)
	}
	if completedExerciseSets(e) != 2 {
		t.Fatalf("blank must use snapshot, not added rows: %#v", e.Sets)
	}
	for _, count := range []string{"0", "-1", "1.5", "oops", "10001"} {
		before = append([]domain.SessionSet(nil), e.Sets...)
		if err := applyEMOM(&e, &emomEntryState{Reps: "10", Count: count}, true); err == nil || !reflect.DeepEqual(before, e.Sets) {
			t.Fatalf("accepted invalid count %q", count)
		}
	}
}

func TestEMOMAverageExcludesSkippedAndUnlogged(t *testing.T) {
	e := sampleEMOM()
	e.Sets[0].Reps = 5
	e.Sets[2].Reps = 8
	e.Sets = append(e.Sets, domain.SessionSet{Reps: 0}, domain.SessionSet{Reps: 100, Deleted: true})
	if got := emomStats(e); got != "2 minutes done · 6.5 avg reps/min" {
		t.Fatal(got)
	}
	for i := range e.Sets {
		e.Sets[i].Reps = 0
	}
	if got := emomStats(e); got != "0 minutes done · — avg reps/min" {
		t.Fatal(got)
	}
}

func TestPendingEMOMRowsRoundTrip(t *testing.T) {
	e := sampleEMOM()
	e.IntervalMinutes = 3
	for i := range e.Sets {
		e.Sets[i].Position = i + 1
	}
	appendEMOMMinute(&e)
	e.Sets[3].Reps = 12
	values := url.Values{"performed_on": {"2026-08-31"}, "emom_pending_7": {"1"}}
	for _, set := range e.Sets {
		key := sessionSetKey(set)
		values.Set("reps_"+key, fmt.Sprint(set.Reps))
		values.Set("weight_"+key, fmt.Sprint(set.WeightKG))
		if set.Skipped {
			values.Set("skipped_"+key, "on")
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	_ = r.ParseForm()
	exercises := []domain.SessionExercise{e}
	exercises[0].Sets = exercises[0].Sets[:3]
	updates, err := overlaySessionForm(r, &domain.Session{}, exercises)
	if err != nil {
		t.Fatal(err)
	}
	if len(updates) != 4 || updates[3].ID != 0 || updates[3].SessionExerciseID != 7 || updates[3].Reps != 12 || updates[3].Minute != 11 {
		t.Fatalf("pending row: %#v", updates)
	}
}

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
	for _, want := range []string{`id="session-form"`, `hx-include="#session-form"`, `hx-sync="#session-form:queue all"`, `hx-disabled-elt="#session-fields"`, `formaction="/sessions/42/exercises/7/emom"`, `name="emom_count_7"`, `name="emom_present_7"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %s", want)
		}
	}
	if strings.Contains(html, `form="emom-form-`) || strings.Contains(html, `<noscript>`) {
		t.Fatal("bulk entry must also submit the full session without HTMX")
	}
	if strings.Contains(html, "hx-on:") || strings.Contains(html, "initializeEMOM") {
		t.Fatal("EMOM must not require custom event scripts")
	}
}

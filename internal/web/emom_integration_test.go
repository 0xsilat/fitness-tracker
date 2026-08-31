package web

import (
	"context"
	"fmt"
	"html"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/local/fitness-tracker/internal/domain"
	"github.com/local/fitness-tracker/internal/store"
)

func emomTestStore(t *testing.T) *store.Store {
	t.Helper()
	connection := os.Getenv("TEST_DATABASE_URL")
	if connection == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, connection)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("emom_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(connection)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	db := store.New(pool)
	if err = db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return db
}

func emomFixture(t *testing.T, db *store.Store) (domain.Session, []domain.SessionExercise) {
	t.Helper()
	ctx := context.Background()
	routine, err := db.CreateRoutine(ctx, "EMOM regression", "Disposable test")
	if err != nil {
		t.Fatal(err)
	}
	workout, err := db.CreateWorkout(ctx, routine, "Intervals")
	if err != nil {
		t.Fatal(err)
	}
	exercises, err := db.Exercises(ctx, false)
	if err != nil || len(exercises) == 0 {
		t.Fatalf("exercise fixture: %v", err)
	}
	if err = db.AddMovement(ctx, workout, exercises[0].ID, "emom", 0, 0, 10, 2, 8, 3); err != nil {
		t.Fatal(err)
	}
	if err = db.AddMovement(ctx, workout, exercises[0].ID, "sets_reps", 1, 0, 10, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	id, err := db.StartSession(ctx, workout, time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	session, entries, err := db.Session(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return session, entries
}

func emomForm(session domain.Session, exercises []domain.SessionExercise) url.Values {
	form := url.Values{"performed_on": {inputDate(session.PerformedOn)}, "notes": {"unsaved notes"}, "rpe": {"7"}}
	for _, e := range exercises {
		for _, set := range e.Sets {
			key := sessionSetKey(set)
			form.Set("reps_"+key, fmt.Sprint(set.Reps))
			if e.Mode == "weighted" {
				form.Set("weight_"+key, fmt.Sprint(set.WeightKG))
			}
			if set.Skipped {
				form.Set("skipped_"+key, "on")
			}
		}
	}
	return form
}

// Read the inputs actually returned by Templ, including temporary row keys.
func mergeEMOMInputs(form url.Values, markup string) {
	inputs := regexp.MustCompile(`<input\b[^>]*>`).FindAllString(markup, -1)
	attribute := regexp.MustCompile(`([\w-]+)="([^"]*)"`)
	for _, input := range inputs {
		attrs := map[string]string{}
		for _, pair := range attribute.FindAllStringSubmatch(input, -1) {
			attrs[pair[1]] = html.UnescapeString(pair[2])
		}
		name := attrs["name"]
		if name == "" {
			continue
		}
		if attrs["type"] == "checkbox" {
			form.Del(name)
			if strings.Contains(input, " checked") {
				form.Set(name, "on")
			}
		} else {
			form.Set(name, attrs["value"])
		}
	}
}

func TestEMOMSubmissionIntegration(t *testing.T) {
	for _, hx := range []bool{false, true} {
		t.Run(fmt.Sprint("htmx=", hx), func(t *testing.T) {
			db := emomTestStore(t)
			session, entries := emomFixture(t, db)
			ctx := context.Background()
			handler := New(db)
			path := fmt.Sprintf("/sessions/%d/exercises/%d/emom", session.ID, entries[0].ID)
			countKey := fmt.Sprintf("emom_count_%d", entries[0].ID)
			repsKey := fmt.Sprintf("emom_reps_%d", entries[0].ID)
			post := func(path string, form url.Values) *httptest.ResponseRecorder {
				t.Helper()
				r := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
				r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				if hx {
					r.Header.Set("HX-Request", "true")
				}
				w := httptest.NewRecorder()
				handler.ServeHTTP(w, r)
				if w.Code >= 400 {
					t.Fatalf("POST %s: %d %s", path, w.Code, w.Body.String())
				}
				return w
			}
			read := func() (domain.Session, []domain.SessionExercise) {
				t.Helper()
				s, e, err := db.Session(ctx, session.ID)
				if err != nil {
					t.Fatal(err)
				}
				return s, e
			}
			form := emomForm(session, entries)
			form.Set("reps_"+sessionSetKey(entries[1].Sets[0]), "7")
			form.Set(countKey, "2")
			form.Set(repsKey, "8")
			form.Set("emom_action", "apply")
			w := post(path, form)
			if !strings.Contains(w.Body.String(), "2 minutes done · 8.0 avg reps/min") {
				t.Fatal("missing average")
			}
			session, entries = read()
			if session.CompletedSets != 3 || session.Notes != "unsaved notes" || entries[1].Sets[0].Reps != 7 {
				t.Fatal("lost other session inputs")
			}
			if err := db.CompleteSession(ctx, session.ID); err != nil {
				t.Fatalf("short EMOM cannot complete: %v", err)
			}
			session, entries = read()
			form = emomForm(session, entries)
			form.Set(countKey, "5")
			form.Set(repsKey, "8")
			form.Set("emom_action", "apply")
			w = post(path, form)
			_, stored := read()
			if len(stored[0].Sets) != 3 {
				t.Fatal("completed extension saved before Save changes")
			}
			mergeEMOMInputs(form, w.Body.String())
			if form.Get(fmt.Sprintf("emom_pending_%d", entries[0].ID)) != "2" {
				t.Fatal("missing pending rows")
			}
			post(fmt.Sprintf("/sessions/%d/update", session.ID), form)
			session, entries = read()
			if len(entries[0].Sets) != 5 || entries[0].Sets[4].Minute != 14 || completedExerciseSets(entries[0]) != 5 {
				t.Fatalf("extension not committed: %#v", entries[0])
			}
			form = emomForm(session, entries)
			form.Set(countKey, "1")
			form.Set(repsKey, "9")
			form.Set("emom_action", "apply")
			w = post(path, form)
			if !strings.Contains(w.Body.String(), "Replace values") {
				t.Fatal("missing overwrite confirmation")
			}
			form.Set("emom_action", "cancel")
			post(path, form)
			_, stored = read()
			if completedExerciseSets(stored[0]) != 5 || stored[0].Sets[0].Reps != 8 {
				t.Fatal("cancel changed stored values")
			}
			form.Set("emom_action", "confirm")
			w = post(path, form)
			mergeEMOMInputs(form, w.Body.String())
			post(fmt.Sprintf("/sessions/%d/update", session.ID), form)
			session, entries = read()
			if completedExerciseSets(entries[0]) != 1 {
				t.Fatal("confirmed shorter count not persisted")
			}
			form = emomForm(session, entries)
			form.Set("emom_action", "remove:"+sessionSetKey(entries[0].Sets[0]))
			w = post(path, form)
			_, stored = read()
			if len(stored[0].Sets) != 5 {
				t.Fatal("delete saved before Save changes")
			}
			mergeEMOMInputs(form, w.Body.String())
			post(fmt.Sprintf("/sessions/%d/update", session.ID), form)
			_, entries = read()
			if len(entries[0].Sets) != 4 {
				t.Fatal("deferred delete not persisted")
			}
			// A failed insert must roll back metadata and all previous updates.
			before := entries[0].Sets[0].Reps
			updates := []store.SetUpdate{{ID: entries[0].Sets[0].ID, Reps: 99}, {SessionExerciseID: entries[0].ID, Position: 999, Minute: 99, Reps: 9}}
			if err := db.UpdateCompletedSession(ctx, session.ID, session.PerformedOn, "must roll back", nil, updates); err == nil {
				t.Fatal("accepted stale insert position")
			}
			session, stored = read()
			if session.Notes == "must roll back" || stored[0].Sets[0].Reps != before {
				t.Fatal("partial transaction persisted")
			}
		})
	}
}

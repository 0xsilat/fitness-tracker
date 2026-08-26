CREATE TABLE IF NOT EXISTS schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS exercises (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    mode text NOT NULL CHECK (mode IN ('weighted', 'bodyweight')),
    archived boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS exercises_name_unique ON exercises (lower(name));

CREATE TABLE IF NOT EXISTS routines (
    id bigserial PRIMARY KEY,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    active boolean NOT NULL DEFAULT false,
    archived boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS one_active_routine ON routines (active) WHERE active;

CREATE TABLE IF NOT EXISTS workouts (
    id bigserial PRIMARY KEY,
    routine_id bigint NOT NULL REFERENCES routines(id) ON DELETE CASCADE,
    name text NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    format text NOT NULL CHECK (format IN ('sets_reps','emom','ascending_pyramid','descending_pyramid')),
    prescription jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (routine_id, position)
);

CREATE TABLE IF NOT EXISTS sessions (
    id bigserial PRIMARY KEY,
    routine_id bigint NOT NULL REFERENCES routines(id),
    workout_id bigint NOT NULL REFERENCES workouts(id),
    routine_name text NOT NULL,
    workout_name text NOT NULL,
    format text NOT NULL,
    status text NOT NULL CHECK (status IN ('draft','completed')),
    prescription_snapshot jsonb NOT NULL,
    notes text NOT NULL DEFAULT '',
    rpe integer CHECK (rpe BETWEEN 1 AND 10),
    started_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz
);
CREATE UNIQUE INDEX IF NOT EXISTS one_draft_per_workout ON sessions(workout_id) WHERE status = 'draft';
CREATE INDEX IF NOT EXISTS sessions_routine_completed ON sessions(routine_id, completed_at DESC) WHERE status = 'completed';

CREATE TABLE IF NOT EXISTS session_exercises (
    id bigserial PRIMARY KEY,
    session_id bigint NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    exercise_id bigint NOT NULL REFERENCES exercises(id),
    exercise_name text NOT NULL,
    tracking_mode text NOT NULL CHECK (tracking_mode IN ('weighted','bodyweight')),
    position integer NOT NULL CHECK (position > 0)
);

CREATE TABLE IF NOT EXISTS session_sets (
    id bigserial PRIMARY KEY,
    session_exercise_id bigint NOT NULL REFERENCES session_exercises(id) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position > 0),
    reps integer NOT NULL DEFAULT 0 CHECK (reps >= 0),
    weight_kg numeric(8,2) NOT NULL DEFAULT 0 CHECK (weight_kg >= 0),
    skipped boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS session_sets_exercise ON session_sets(session_exercise_id, position);

INSERT INTO exercises(name, mode) VALUES
 ('Back Squat','weighted'), ('Bench Press','weighted'), ('Deadlift','weighted'),
 ('Overhead Press','weighted'), ('Barbell Row','weighted'), ('Dumbbell Curl','weighted'),
 ('Pull-up','bodyweight'), ('Push-up','bodyweight'), ('Dip','bodyweight'),
 ('Plank','bodyweight'), ('Burpee','bodyweight'), ('Bodyweight Squat','bodyweight')
ON CONFLICT DO NOTHING;


ALTER TABLE exercises DROP CONSTRAINT exercises_mode_check;
ALTER TABLE exercises ADD CONSTRAINT exercises_mode_check CHECK (mode IN ('weighted','bodyweight','cardio'));

CREATE TABLE cardio_sessions (
    id bigserial PRIMARY KEY,
    exercise_id bigint NOT NULL REFERENCES exercises(id),
    exercise_name text NOT NULL,
    performed_on date NOT NULL,
    duration_minutes numeric(9,1) NOT NULL CHECK (duration_minutes > 0 AND duration_minutes < 100000000),
    notes text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX cardio_sessions_date ON cardio_sessions(performed_on DESC, created_at DESC);
CREATE INDEX cardio_sessions_exercise ON cardio_sessions(exercise_id, performed_on);

INSERT INTO exercises(name,mode) VALUES
 ('Treadmill','cardio'), ('Incline treadmill','cardio'), ('Running','cardio'),
 ('Jumping rope','cardio'), ('Walking','cardio'), ('Cycling','cardio'),
 ('Stationary bike','cardio'), ('Rowing','cardio'), ('Elliptical','cardio'), ('Swimming','cardio')
ON CONFLICT DO NOTHING;

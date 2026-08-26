ALTER TABLE session_exercises ADD COLUMN IF NOT EXISTS interval_minutes integer NOT NULL DEFAULT 0 CHECK (interval_minutes >= 0);
ALTER TABLE session_sets ADD COLUMN IF NOT EXISTS target_reps integer CHECK (target_reps > 0);
ALTER TABLE session_sets ADD COLUMN IF NOT EXISTS planned_minute integer NOT NULL DEFAULT 0 CHECK (planned_minute >= 0);

UPDATE session_sets SET target_reps = NULLIF(reps, 0) WHERE target_reps IS NULL;
UPDATE session_exercises SET workout_format = 'sets_reps' WHERE workout_format IN ('ascending_pyramid','descending_pyramid');
UPDATE session_exercises SET interval_minutes = 1 WHERE workout_format = 'emom' AND interval_minutes = 0;
UPDATE sessions SET format = 'mixed' WHERE format <> 'mixed';

UPDATE workouts w
SET prescription = jsonb_set(
    w.prescription,
    '{movements}',
    COALESCE((
        SELECT jsonb_agg(
            CASE
                WHEN movement->>'format' IN ('ascending_pyramid','descending_pyramid')
                    THEN jsonb_set(movement, '{format}', '"sets_reps"'::jsonb, true)
                WHEN movement->>'format' = 'emom' AND NOT (movement ? 'interval_minutes')
                    THEN jsonb_set(movement, '{interval_minutes}', '1'::jsonb, true)
                ELSE movement
            END
            ORDER BY ordinal
        )
        FROM jsonb_array_elements(COALESCE(w.prescription->'movements','[]'::jsonb)) WITH ORDINALITY AS items(movement, ordinal)
    ), '[]'::jsonb),
    true
);

UPDATE sessions s
SET prescription_snapshot = jsonb_set(
    s.prescription_snapshot,
    '{movements}',
    COALESCE((
        SELECT jsonb_agg(
            CASE
                WHEN movement->>'format' IN ('ascending_pyramid','descending_pyramid')
                    THEN jsonb_set(movement, '{format}', '"sets_reps"'::jsonb, true)
                WHEN movement->>'format' = 'emom' AND NOT (movement ? 'interval_minutes')
                    THEN jsonb_set(movement, '{interval_minutes}', '1'::jsonb, true)
                ELSE movement
            END
            ORDER BY ordinal
        )
        FROM jsonb_array_elements(COALESCE(s.prescription_snapshot->'movements','[]'::jsonb)) WITH ORDINALITY AS items(movement, ordinal)
    ), '[]'::jsonb),
    true
);

ALTER TABLE workouts DROP CONSTRAINT IF EXISTS workouts_format_check;
ALTER TABLE workouts ADD CONSTRAINT workouts_format_check CHECK (format IN ('sets_reps','emom','mixed'));
ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_format_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_format_check CHECK (format IN ('sets_reps','emom','mixed'));
ALTER TABLE session_exercises DROP CONSTRAINT IF EXISTS session_exercises_workout_format_check;
ALTER TABLE session_exercises ADD CONSTRAINT session_exercises_workout_format_check CHECK (workout_format IN ('sets_reps','emom'));

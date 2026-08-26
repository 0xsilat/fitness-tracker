ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_workout_id_fkey;
ALTER TABLE sessions ALTER COLUMN workout_id DROP NOT NULL;
ALTER TABLE sessions
    ADD CONSTRAINT sessions_workout_id_fkey
    FOREIGN KEY (workout_id) REFERENCES workouts(id) ON DELETE SET NULL;

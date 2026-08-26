ALTER TABLE workouts DROP CONSTRAINT IF EXISTS workouts_format_check;
ALTER TABLE workouts ADD CONSTRAINT workouts_format_check CHECK (format IN ('sets_reps','emom','ascending_pyramid','descending_pyramid','mixed'));
ALTER TABLE workouts ALTER COLUMN format SET DEFAULT 'mixed';

ALTER TABLE sessions DROP CONSTRAINT IF EXISTS sessions_format_check;
ALTER TABLE sessions ADD CONSTRAINT sessions_format_check CHECK (format IN ('sets_reps','emom','ascending_pyramid','descending_pyramid','mixed'));

ALTER TABLE session_exercises ADD COLUMN IF NOT EXISTS workout_format text NOT NULL DEFAULT 'sets_reps';
ALTER TABLE session_exercises ADD COLUMN IF NOT EXISTS duration_minutes integer NOT NULL DEFAULT 0 CHECK (duration_minutes >= 0);
ALTER TABLE session_exercises DROP CONSTRAINT IF EXISTS session_exercises_workout_format_check;
ALTER TABLE session_exercises ADD CONSTRAINT session_exercises_workout_format_check CHECK (workout_format IN ('sets_reps','emom','ascending_pyramid','descending_pyramid'));

UPDATE session_exercises se SET workout_format = s.format
FROM sessions s
WHERE s.id = se.session_id AND s.format <> 'mixed';

UPDATE workouts SET format = 'mixed';

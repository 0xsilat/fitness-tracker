ALTER TABLE sessions ADD COLUMN IF NOT EXISTS performed_on date;
UPDATE sessions SET performed_on = coalesce(completed_at, started_at)::date WHERE performed_on IS NULL;
ALTER TABLE sessions ALTER COLUMN performed_on SET NOT NULL;
ALTER TABLE sessions ALTER COLUMN performed_on SET DEFAULT current_date;
CREATE INDEX IF NOT EXISTS sessions_routine_performed ON sessions(routine_id, performed_on DESC) WHERE status = 'completed';

DROP TABLE IF EXISTS simulation_artifacts;
DROP TABLE IF EXISTS simulation_replications;

DROP INDEX IF EXISTS simulation_jobs_run_id_idx;
DROP INDEX IF EXISTS simulation_jobs_project_created_idx;
DROP INDEX IF EXISTS simulation_jobs_user_active_idx;
DROP INDEX IF EXISTS simulation_jobs_active_idx;

ALTER TABLE simulation_jobs
    DROP CONSTRAINT IF EXISTS simulation_jobs_progress_check,
    DROP CONSTRAINT IF EXISTS simulation_jobs_status_check,
    DROP COLUMN IF EXISTS heartbeat_at,
    DROP COLUMN IF EXISTS finished_at,
    DROP COLUMN IF EXISTS started_at,
    DROP COLUMN IF EXISTS replications_done,
    DROP COLUMN IF EXISTS replications_total,
    DROP COLUMN IF EXISTS max_attempts,
    DROP COLUMN IF EXISTS config,
    DROP COLUMN IF EXISTS user_id,
    DROP CONSTRAINT IF EXISTS simulation_jobs_run_id_fkey;

ALTER TABLE simulation_jobs
    ADD CONSTRAINT simulation_jobs_run_id_fkey FOREIGN KEY (run_id) REFERENCES calculation_runs (id);

DROP INDEX IF EXISTS calculation_runs_project_kind_created_idx;
CREATE INDEX calculation_runs_project_id_created_idx
    ON calculation_runs (project_id, created_at DESC);

ALTER TABLE calculation_runs
    DROP CONSTRAINT IF EXISTS calculation_runs_status_check,
    DROP CONSTRAINT IF EXISTS calculation_runs_kind_check,
    DROP COLUMN IF EXISTS kind;

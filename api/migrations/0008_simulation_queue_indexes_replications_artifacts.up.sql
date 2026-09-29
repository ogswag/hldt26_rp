ALTER TABLE calculation_runs
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'calculation';

ALTER TABLE calculation_runs
    ADD CONSTRAINT calculation_runs_kind_check CHECK (kind IN ('calculation', 'simulation')),
    ADD CONSTRAINT calculation_runs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled'));

-- History lists filter by kind; the old index could not serve that filter.
DROP INDEX calculation_runs_project_id_created_idx;
CREATE INDEX calculation_runs_project_kind_created_idx
    ON calculation_runs (project_id, kind, created_at DESC);

ALTER TABLE simulation_jobs
    DROP CONSTRAINT simulation_jobs_run_id_fkey;

ALTER TABLE simulation_jobs
    ADD CONSTRAINT simulation_jobs_run_id_fkey
        FOREIGN KEY (run_id) REFERENCES calculation_runs (id) ON DELETE CASCADE,
    ADD COLUMN user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    ADD COLUMN config JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3,
    ADD COLUMN replications_total INTEGER NOT NULL DEFAULT 1,
    ADD COLUMN replications_done INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN started_at TIMESTAMPTZ,
    ADD COLUMN finished_at TIMESTAMPTZ,
    ADD COLUMN heartbeat_at TIMESTAMPTZ,
    ADD CONSTRAINT simulation_jobs_status_check CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'canceled')),
    ADD CONSTRAINT simulation_jobs_progress_check CHECK (progress_pct BETWEEN 0 AND 100);

-- Workers claim the oldest queued job and the janitor scans running ones.
CREATE INDEX simulation_jobs_active_idx
    ON simulation_jobs (status, created_at)
    WHERE status IN ('queued', 'running');

-- Quota checks count active jobs per user.
CREATE INDEX simulation_jobs_user_active_idx
    ON simulation_jobs (user_id)
    WHERE status IN ('queued', 'running');

-- Project history and the one-active-job-per-project quota.
CREATE INDEX simulation_jobs_project_created_idx
    ON simulation_jobs (project_id, created_at DESC);

CREATE INDEX simulation_jobs_run_id_idx
    ON simulation_jobs (run_id);

CREATE TABLE simulation_replications (
    run_id UUID NOT NULL REFERENCES calculation_runs (id) ON DELETE CASCADE,
    idx INTEGER NOT NULL,
    seed BIGINT NOT NULL,
    status TEXT NOT NULL,
    metrics JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (run_id, idx)
);

CREATE TABLE simulation_artifacts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES calculation_runs (id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    encoding TEXT NOT NULL,
    data BYTEA NOT NULL,
    size_bytes INTEGER NOT NULL,
    raw_bytes INTEGER NOT NULL,
    event_count INTEGER NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT false,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (run_id, kind)
);

-- Retention cleanup deletes expired, unpinned journals.
CREATE INDEX simulation_artifacts_expires_idx
    ON simulation_artifacts (expires_at)
    WHERE NOT pinned;

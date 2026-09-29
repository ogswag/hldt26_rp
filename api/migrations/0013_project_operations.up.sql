-- Operation log for collaborative editing (docs/adr/0003-operations-and-sync.md).
ALTER TABLE projects ADD COLUMN draft_seq BIGINT NOT NULL DEFAULT 0;
ALTER TABLE calculation_runs ADD COLUMN draft_seq BIGINT;

CREATE TABLE project_operations (
    id BIGSERIAL PRIMARY KEY,
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    seq BIGINT,
    tx_id UUID NOT NULL,
    client_id TEXT NOT NULL,
    actor_user_id UUID REFERENCES users (id) ON DELETE SET NULL,
    label TEXT NOT NULL DEFAULT '',
    ops JSONB NOT NULL,
    rejected_reason TEXT,
    rejected_details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, tx_id)
);
CREATE UNIQUE INDEX project_operations_seq_idx ON project_operations (project_id, seq) WHERE seq IS NOT NULL;
CREATE INDEX project_operations_created_idx ON project_operations (created_at);

-- Operations carry exact numbers; a scaled column would round what the client already shows.
ALTER TABLE variant_fleet_items ALTER COLUMN price_override_rub TYPE NUMERIC;
ALTER TABLE shared_cost_items ALTER COLUMN rub TYPE NUMERIC;
ALTER TABLE assumption_sets ALTER COLUMN vat_rate TYPE NUMERIC, ALTER COLUMN labor_cash_share TYPE NUMERIC;

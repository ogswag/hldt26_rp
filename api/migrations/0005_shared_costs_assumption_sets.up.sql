CREATE TABLE shared_cost_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    bucket TEXT NOT NULL,
    rub NUMERIC(15, 2) NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    UNIQUE (project_id, code)
);

CREATE INDEX shared_cost_items_project_id_sort_idx
    ON shared_cost_items (project_id, sort_order);

CREATE TABLE assumption_sets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT false,
    vat_rate NUMERIC(6, 4) NOT NULL DEFAULT 0.20,
    prices_include_vat BOOLEAN NOT NULL DEFAULT true,
    vat_recoverable BOOLEAN NOT NULL DEFAULT false,
    labor_cash_share NUMERIC(6, 4) NOT NULL DEFAULT 1,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX assumption_sets_project_id_sort_idx
    ON assumption_sets (project_id, sort_order);

ALTER TABLE assumption_sets ALTER COLUMN vat_rate TYPE NUMERIC(6, 4), ALTER COLUMN labor_cash_share TYPE NUMERIC(6, 4);
ALTER TABLE shared_cost_items ALTER COLUMN rub TYPE NUMERIC(15, 2);
ALTER TABLE variant_fleet_items ALTER COLUMN price_override_rub TYPE NUMERIC(15, 2);

DROP TABLE project_operations;
ALTER TABLE calculation_runs DROP COLUMN draft_seq;
ALTER TABLE projects DROP COLUMN draft_seq;

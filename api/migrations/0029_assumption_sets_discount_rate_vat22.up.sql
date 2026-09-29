-- Discount rate of NPV, IRR and discounted payback. VAT is 22% since 2026-01-01: sets still at the old 20% follow it.
ALTER TABLE assumption_sets ADD COLUMN discount_rate NUMERIC NOT NULL DEFAULT 0.15;
ALTER TABLE assumption_sets ALTER COLUMN vat_rate SET DEFAULT 0.22;
UPDATE assumption_sets SET vat_rate = 0.22 WHERE vat_rate = 0.20;

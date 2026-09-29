-- Fractional order keys for operations (contracts/ops). sort_order stays as the rank of the key.
CREATE OR REPLACE FUNCTION pg_temp.rank_order_key(rn INTEGER)
RETURNS TEXT
LANGUAGE sql
IMMUTABLE
AS $$
    SELECT CASE
        WHEN rn < 62 THEN 'a' || substr(d, rn + 1, 1)
        ELSE 'b' || substr(d, (rn - 62) / 62 + 1, 1) || substr(d, (rn - 62) % 62 + 1, 1)
    END
    FROM (SELECT '0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz'::text AS d) digits
$$;

ALTER TABLE project_processes ADD COLUMN order_key TEXT NOT NULL DEFAULT '';
ALTER TABLE solution_variants ADD COLUMN order_key TEXT NOT NULL DEFAULT '';
ALTER TABLE variant_fleet_items ADD COLUMN order_key TEXT NOT NULL DEFAULT '';
ALTER TABLE shared_cost_items ADD COLUMN order_key TEXT NOT NULL DEFAULT '';
ALTER TABLE assumption_sets ADD COLUMN order_key TEXT NOT NULL DEFAULT '';

UPDATE project_processes t SET order_key = pg_temp.rank_order_key(r.rn)
FROM (
    SELECT id, (row_number() OVER (PARTITION BY project_id ORDER BY sort_order, code) - 1)::int AS rn
    FROM project_processes
) r
WHERE r.id = t.id;

UPDATE solution_variants t SET order_key = pg_temp.rank_order_key(r.rn)
FROM (
    SELECT id, (row_number() OVER (PARTITION BY project_id ORDER BY sort_order, created_at, id) - 1)::int AS rn
    FROM solution_variants
) r
WHERE r.id = t.id;

UPDATE variant_fleet_items t SET order_key = pg_temp.rank_order_key(r.rn)
FROM (
    SELECT id, (row_number() OVER (PARTITION BY variant_id ORDER BY sort_order, id) - 1)::int AS rn
    FROM variant_fleet_items
) r
WHERE r.id = t.id;

UPDATE shared_cost_items t SET order_key = pg_temp.rank_order_key(r.rn)
FROM (
    SELECT id, (row_number() OVER (PARTITION BY project_id ORDER BY sort_order, code) - 1)::int AS rn
    FROM shared_cost_items
) r
WHERE r.id = t.id;

UPDATE assumption_sets t SET order_key = pg_temp.rank_order_key(r.rn)
FROM (
    SELECT id, (row_number() OVER (PARTITION BY project_id ORDER BY sort_order, name) - 1)::int AS rn
    FROM assumption_sets
) r
WHERE r.id = t.id;

DROP FUNCTION pg_temp.rank_order_key(INTEGER);

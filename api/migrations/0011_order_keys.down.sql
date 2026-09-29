ALTER TABLE project_processes DROP COLUMN IF EXISTS order_key;
ALTER TABLE solution_variants DROP COLUMN IF EXISTS order_key;
ALTER TABLE variant_fleet_items DROP COLUMN IF EXISTS order_key;
ALTER TABLE shared_cost_items DROP COLUMN IF EXISTS order_key;
ALTER TABLE assumption_sets DROP COLUMN IF EXISTS order_key;

-- Fleet sizing and cost rates a set may fix. NULL means the norm applies.
ALTER TABLE assumption_sets
    ADD COLUMN utilization NUMERIC CHECK (utilization IS NULL OR (utilization >= 0.01 AND utilization <= 1)),
    ADD COLUMN availability NUMERIC CHECK (availability IS NULL OR (availability >= 0.01 AND availability <= 1)),
    ADD COLUMN reserve NUMERIC CHECK (reserve IS NULL OR (reserve >= 0 AND reserve <= 1)),
    ADD COLUMN service_share NUMERIC CHECK (service_share IS NULL OR (service_share >= 0 AND service_share <= 1)),
    ADD COLUMN delivery_share NUMERIC CHECK (delivery_share IS NULL OR (delivery_share >= 0 AND delivery_share <= 1)),
    ADD COLUMN comm_rub_per_robot_year NUMERIC CHECK (comm_rub_per_robot_year IS NULL OR (comm_rub_per_robot_year >= 0 AND comm_rub_per_robot_year <= 10000000)),
    ADD COLUMN technician_wage_month_rub NUMERIC CHECK (technician_wage_month_rub IS NULL OR (technician_wage_month_rub >= 0 AND technician_wage_month_rub <= 10000000));

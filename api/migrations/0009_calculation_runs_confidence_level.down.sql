ALTER TABLE calculation_runs
    DROP CONSTRAINT IF EXISTS calculation_runs_confidence_level_check,
    DROP COLUMN IF EXISTS confidence_level;

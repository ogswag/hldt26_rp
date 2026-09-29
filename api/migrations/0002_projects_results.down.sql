ALTER TABLE projects
    DROP COLUMN IF EXISTS calc_seed,
    DROP COLUMN IF EXISTS results;

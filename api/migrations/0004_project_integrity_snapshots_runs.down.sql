DROP TABLE IF EXISTS simulation_jobs;
DROP TABLE IF EXISTS calculation_results;
DROP TABLE IF EXISTS calculation_runs;
DROP TABLE IF EXISTS financing_scenarios;
DROP TABLE IF EXISTS variant_fleet_items;
DROP TABLE IF EXISTS solution_variants;
DROP TABLE IF EXISTS project_processes;
DROP TABLE IF EXISTS project_versions;
DROP TABLE IF EXISTS project_drafts;

ALTER TABLE projects
    DROP COLUMN IF EXISTS input_hash,
    DROP COLUMN IF EXISTS catalog_revision_id,
    DROP COLUMN IF EXISTS current_run_id,
    DROP COLUMN IF EXISTS deleted_at,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS status;

DROP TABLE IF EXISTS catalog_revisions;

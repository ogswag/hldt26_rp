ALTER TABLE calculation_runs
    ADD COLUMN confidence_level TEXT NOT NULL DEFAULT 'preliminary';

ALTER TABLE calculation_runs
    ADD CONSTRAINT calculation_runs_confidence_level_check
        CHECK (confidence_level IN ('preliminary', 'configured', 'calibrated', 'validated'));

-- sim-v2 runs on a project map were configured; the level was only kept in the result summary.
UPDATE calculation_runs r
SET confidence_level = 'configured'
FROM project_versions v
WHERE r.kind = 'simulation'
  AND v.id = r.project_version_id
  AND jsonb_typeof(v.snapshot->'draft'->'map') = 'object';

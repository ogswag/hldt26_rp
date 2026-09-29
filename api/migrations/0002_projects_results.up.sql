ALTER TABLE projects
    ADD COLUMN results JSONB,
    ADD COLUMN calc_seed INTEGER;

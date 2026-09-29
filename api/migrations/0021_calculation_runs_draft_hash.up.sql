-- Staleness compares draft_hash: the run's snapshot hashed the way drafts are hashed today. input_hash stays as
-- recorded for reports. NULL means not hashed yet; the API fills it on the project's next draft save.
ALTER TABLE calculation_runs ADD COLUMN draft_hash TEXT;

-- An archived robot leaves the lists and matching; fleets and runs that name it still load it.
ALTER TABLE solutions ADD COLUMN archived_at TIMESTAMPTZ;

-- Startup seeding fills gaps only in robots nobody has edited in the admin catalog.
ALTER TABLE solutions ADD COLUMN edited_at TIMESTAMPTZ;

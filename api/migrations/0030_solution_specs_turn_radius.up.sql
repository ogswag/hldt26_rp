-- The matching rule for turning room reads the radius a robot needs to turn, when a vendor states it.
ALTER TABLE solution_specs ADD COLUMN turn_radius_mm NUMERIC;

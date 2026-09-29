-- Robots now come from the recommendation, not from hand-built variants. Variants keep their names and financing;
-- their fleet is cleared, and the projects that had one are marked stale so the next visit recalculates them.
UPDATE projects SET input_hash = ''
WHERE id IN (
    SELECT v.project_id FROM solution_variants v JOIN variant_fleet_items f ON f.variant_id = v.id
);

UPDATE project_drafts d
SET document = jsonb_set(
        d.document, '{variants}',
        (SELECT coalesce(jsonb_agg(e.v || '{"fleet": []}'::jsonb ORDER BY e.ord), '[]'::jsonb)
         FROM jsonb_array_elements(d.document -> 'variants') WITH ORDINALITY AS e (v, ord))),
    input_hash = ''
WHERE jsonb_typeof(d.document -> 'variants') = 'array'
  AND EXISTS (
    SELECT 1 FROM jsonb_array_elements(d.document -> 'variants') v
    WHERE jsonb_typeof(v -> 'fleet') = 'array' AND jsonb_array_length(v -> 'fleet') > 0
  );

DELETE FROM variant_fleet_items;

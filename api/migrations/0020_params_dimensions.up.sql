-- pallet_size_mm and unit_size_mm change from "1200x800x1600" strings to {length, width, height} objects in mm.
-- Separators: x, X, *, the multiplication sign and Cyrillic x. A string that does not parse stays as it was,
-- and the form asks for the value again.
UPDATE projects p
SET params = p.params || d.fixed
FROM (
    SELECT pr.id, jsonb_object_agg(k.key, jsonb_build_object(
        'length', replace(m.parts[1], ',', '.')::numeric,
        'width', replace(m.parts[2], ',', '.')::numeric,
        'height', replace(m.parts[3], ',', '.')::numeric)) AS fixed
    FROM projects pr
    CROSS JOIN unnest(ARRAY['pallet_size_mm', 'unit_size_mm']) AS k (key)
    CROSS JOIN LATERAL regexp_match(pr.params ->> k.key,
        '^\s*([0-9]+(?:[.,][0-9]+)?)\s*[xX*\u00d7\u0445\u0425]\s*([0-9]+(?:[.,][0-9]+)?)\s*[xX*\u00d7\u0445\u0425]\s*([0-9]+(?:[.,][0-9]+)?)\s*$'
    ) AS m (parts)
    WHERE jsonb_typeof(pr.params -> k.key) = 'string' AND m.parts IS NOT NULL
    GROUP BY pr.id
) d
WHERE p.id = d.id;

UPDATE project_drafts pd
SET document = jsonb_set(pd.document, '{params}', p.params)
FROM projects p
WHERE p.id = pd.project_id
  AND jsonb_typeof(pd.document -> 'params') = 'object'
  AND (jsonb_typeof(pd.document -> 'params' -> 'pallet_size_mm') = 'string'
    OR jsonb_typeof(pd.document -> 'params' -> 'unit_size_mm') = 'string');

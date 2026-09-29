UPDATE projects p
SET params = p.params || d.fixed
FROM (
    SELECT pr.id, jsonb_object_agg(k.key, to_jsonb(concat_ws('x',
        pr.params -> k.key ->> 'length', pr.params -> k.key ->> 'width', pr.params -> k.key ->> 'height'))) AS fixed
    FROM projects pr
    CROSS JOIN unnest(ARRAY['pallet_size_mm', 'unit_size_mm']) AS k (key)
    WHERE jsonb_typeof(pr.params -> k.key) = 'object'
    GROUP BY pr.id
) d
WHERE p.id = d.id;

UPDATE project_drafts pd
SET document = jsonb_set(pd.document, '{params}', p.params)
FROM projects p
WHERE p.id = pd.project_id
  AND jsonb_typeof(pd.document -> 'params') = 'object'
  AND (jsonb_typeof(pd.document -> 'params' -> 'pallet_size_mm') = 'object'
    OR jsonb_typeof(pd.document -> 'params' -> 'unit_size_mm') = 'object');

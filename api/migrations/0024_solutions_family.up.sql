-- Robot group for colors, icons and filters, from the organizer's «Тип» column. NULL: not known yet.
ALTER TABLE solutions ADD COLUMN family TEXT
    CONSTRAINT solutions_family_code
    CHECK (family IN ('mobile', 'uav', 'ground', 'marine', 'stationary', 'manipulator', 'humanoid', 'software', 'other'));

UPDATE solutions SET family = CASE btrim(COALESCE(raw->>'Тип', ''))
    WHEN 'Мобильные роботы' THEN 'mobile'
    WHEN 'БАС' THEN 'uav'
    WHEN 'Автономные наземные транспортные средства' THEN 'ground'
    WHEN 'Морские роботы' THEN 'marine'
    WHEN 'Стационарные роботизированные системы' THEN 'stationary'
    WHEN 'Роботы-манипуляторы' THEN 'manipulator'
    WHEN 'Мобильные манипуляторы' THEN 'manipulator'
    WHEN 'Антропоморфные роботы' THEN 'humanoid'
    WHEN 'ПО БРС' THEN 'software'
    WHEN 'ПО' THEN 'software'
    WHEN '' THEN CASE kind WHEN 'bas' THEN 'uav' WHEN 'software' THEN 'software' END
    ELSE 'other'
END;

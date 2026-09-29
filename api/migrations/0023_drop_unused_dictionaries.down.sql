CREATE TABLE dictionary_entries (
    kind TEXT NOT NULL,
    code TEXT NOT NULL,
    label TEXT NOT NULL,
    object_type TEXT,
    PRIMARY KEY (kind, code)
);

CREATE TABLE capability_task_rules (
    capability_code TEXT NOT NULL,
    task_code TEXT NOT NULL,
    relation TEXT NOT NULL,
    PRIMARY KEY (capability_code, task_code)
);

INSERT INTO dictionary_entries (kind, code, label, object_type) VALUES
    ('task', 'pallet_inbound', 'Приёмка паллет', 'warehouse'),
    ('task', 'pallet_putaway', 'Размещение паллет', 'warehouse'),
    ('task', 'pallet_outbound', 'Отгрузка паллет', 'warehouse'),
    ('task', 'pallet_move', 'Паллетное перемещение', 'warehouse'),
    ('task', 'piece_pick', 'Мелкоштучный отбор', 'warehouse'),
    ('task', 'cleaning', 'Уборка', 'warehouse'),
    ('capability', 'warehouse_indoor', 'Наземная работа на складе', 'warehouse'),
    ('capability', 'payload_pallet', 'Паллетная нагрузка', 'warehouse'),
    ('capability', 'payload_unit', 'Штучная или контейнерная нагрузка', 'warehouse'),
    ('capability', 'aisle_rated', 'Есть ширина или минимальный проезд', 'warehouse'),
    ('capability', 'temp_rated', 'Есть рабочий диапазон температур', 'warehouse'),
    ('capability', 'cleaning', 'Уборка помещений', 'warehouse'),
    ('capability', 'amr', 'Автономный мобильный робот', 'warehouse'),
    ('capability', 'stacker', 'Штабелёр', 'warehouse'),
    ('capability', 'forklift', 'Погрузчик', 'warehouse');

INSERT INTO capability_task_rules (capability_code, task_code, relation) VALUES
    ('payload_pallet', 'pallet_inbound', 'required'),
    ('payload_pallet', 'pallet_putaway', 'required'),
    ('payload_pallet', 'pallet_outbound', 'required'),
    ('payload_pallet', 'pallet_move', 'required'),
    ('payload_unit', 'piece_pick', 'required'),
    ('cleaning', 'cleaning', 'required');

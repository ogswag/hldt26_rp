# Catalog data

Catalog file is `catalog_export_v4` only. There is no v5 (organizer typo). TTX is not another catalog table.

## Organizer files

| File | Role |
|---|---|
| `Датасет/catalog_export_v4.csv` | Names, vendors, prices. Copied to `data/catalog.csv`. |
| `Датасет/catalog_export_v4.xlsx` | Same catalog. |
| `Датасет/Датасеты_хакатон.xlsx` | Object params (warehouse / airport / hospital). Not robot TTX. |
| `Датасет/Примеры_решений_типы_объектов.docx` | Example TTX used in `robot_specs.json`. |
| `Датасет/ФЦ БАС — Каталог внедрения 2008 1247.pdf` | PDF catalog. Not parsed. |

CSV inventory is produced by `importers.InventoryFromRecords` on `data/catalog.csv`.
Current file: 223 rows, 187 unique ids, 23 repeated ids (36 extra rows).

## Duplicate ids

The organizers ask not to delete or merge duplicates: a repeated id is the same product for another industry,
scenario or price. Every CSV row is its own solution, 223 in all.

- The first row of an id keeps the delivered UUID, so saved fleets and runs stay valid.
- Every later row gets `uuid v5(namespace, id + industry)` (`importers.SplitID`), which does not depend on file order.
  A second row with the same id and industry is refused.
- All rows of a repeated id carry `raw.source_id` (the delivered id). The API shows their industry as `modification`.
- Each row keeps its own `industry`, `scenario`, `cases` and price. `raw.uses` holds that one row.
- The specs seed applies to every row of its id.
- A catalog seeded before rows became solutions is brought up to date at start: the missing rows are added and the
  first row's uses are cut to its own (robots an admin edited keep theirs).

### Price conflicts (each row keeps its own price)

| id | name | first row | other row |
|---|---|---|---|
| `ce44017e-6ada-421d-bbce-6869b33c2cda` | Модель А25-1720 | 2 000 000 | 2 500 000 |
| `fceeacf0-579f-46cc-92ff-2057fea5da7f` | Модель А12-1450 | 2 000 000 | 2 500 000 |
| `38db0187-c455-4601-9ba1-7a1012485e4a` | Патрулирование и охрана | 3 600 000 | 3 000 000 |

## Specs seed

`robot_specs.json` holds 58 robots. `object_types` is written to `solutions.raw.object_types` for process match.

- 19 come from the docx, or from the catalog name (payload only, `assumed`).
- 39 were added and 7 of the 19 extended from public pages on 2026-09-28 (`sourced_at` on the entry).

Research rules: a value goes in only when a public page states it for that model, with the page in
`source_url`. The vendor's page comes first; a distributor, an industry portal or a press report counts when the
vendor has no readable page. Both are stored as `confidence: vendor`, since the numbers are the maker's. A value
the sources disagree on is left out. `gaps` says why a key is empty, `notes` names the other pages and the doubts.
Descriptions are paraphrased in Russian, at most 300 characters. No photos were copied.

About 55 of the 187 catalog robots have researched TTX. The rest were not looked up: their empty fields show in the catalog as «Нет в ТТХ».

On 2026-09-29 the vendor pages of six robots that the demo projects use or rank high were read again: Ronavi SR, M,
H2000 and SD (`ronavi-robotics.ru/catalogue/...`) and MOROS AMR 800 and AMR 1500 (`xn--l1aeahg.xn--p1ai`). They now
carry mass, minimum passage or dimensions and runtime from the vendor, and `confidence: vendor`. Three points to know:

- Ronavi prints "Габариты" without naming the axes. M and H2000 are read as length, width, height, which fits their
  minimum passage. SR is not: its minimum passage (690 mm) fits the first figure of 590x810x870 as the width, so SR
  keeps no dimensions until a page names the axes.
- MOROS gives two runtimes, at average and at maximum load. The seed takes the maximum-load one, and `field_notes`
  says so, so a fleet is not sized on the better figure.
- Ronavi SD minimum passage is 500 mm on the product page, not the 700 mm the seed carried. AMR 800 speed is 2 m/s on
  the vendor page, not the 2,5 m/s of the industry portal.

Not found on any page that could be read: turning radius for every robot, charging time for Ronavi and MOROS, and
dimensions, mass and minimum passage of DMR Carrier P (the vendor site is drawn in the browser and answers a plain
request with no text; the KIIT page has no table). DH:CARGO UNIT and Belka have no product page on droneshub.ru.
The aripix.ru site sells packing cells, not a robot named A1. Yandex rover R 4.0 has no readable page.

Seeding does not overwrite a value that is already stored, so changed values (SD passage, AMR 800 speed and runtime)
reach an existing database only through the admin editor. A clean database (`docker compose down -v`) takes them all.

Navigation (`nav_type`) is Russian text shown as is.

Empty key TTX stays NULL. Matching marks `needs_review`.

PuduBot 2 is not in v4. Extra row `8f3c1e20-6d4a-4b91-9c2e-7b0a11d4e8f2`. Price unknown.

`lifetime_years` and `service_pct_year` are in no source. Left empty. Econ uses an assumed value.

Seeding only fills empty fields and missing robots. It never overwrites a value, and it skips robots an admin has
edited in the catalog (`solutions.edited_at`), so an admin's cleared field stays empty.

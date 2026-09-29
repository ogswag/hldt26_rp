package migrations_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"moscow_hackathon_2026/api/internal/catalogstore"
	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/internal/testdb"
	"moscow_hackathon_2026/api/migrations"
)

func versions(t *testing.T) []uint {
	t.Helper()
	ups, err := fs.Glob(migrations.FS, "*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	var out []uint
	for _, name := range ups {
		n, err := strconv.Atoi(strings.SplitN(path.Base(name), "_", 2)[0])
		if err != nil {
			t.Fatalf("migration %s has no numeric prefix", name)
		}
		down := strings.TrimSuffix(name, ".up.sql") + ".down.sql"
		if _, err := fs.Stat(migrations.FS, down); err != nil {
			t.Fatalf("migration %s has no down file", name)
		}
		out = append(out, uint(n))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	for i, v := range out {
		if v != uint(i+1) {
			t.Fatalf("migration numbers must be contiguous from 1, got %v", out)
		}
	}
	return out
}

func userObjects(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT 'table ' || tablename FROM pg_tables
		WHERE schemaname = 'public' AND tablename <> 'schema_migrations'
		UNION ALL
		SELECT 'function ' || p.proname FROM pg_proc p
		JOIN pg_namespace n ON n.oid = p.pronamespace
		WHERE n.nspname = 'public'
		ORDER BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestMigrationsOnEmptyDatabase(t *testing.T) {
	dsn := testdb.Empty(t)
	all := versions(t)
	m, err := migrations.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()

	for _, v := range all {
		if err := m.Steps(1); err != nil {
			t.Fatalf("up to %d: %v", v, err)
		}
		if err := m.Steps(-1); err != nil {
			t.Fatalf("down from %d: %v", v, err)
		}
		if err := m.Steps(1); err != nil {
			t.Fatalf("up to %d again: %v", v, err)
		}
		got, dirty, err := m.Version()
		if err != nil || dirty || got != v {
			t.Fatalf("after %d: version %d dirty %v err %v", v, got, dirty, err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	if err := m.Down(); err != nil {
		t.Fatalf("down all: %v", err)
	}
	if left := userObjects(t, pool); len(left) != 0 {
		t.Fatalf("down migrations left objects behind: %v", left)
	}
	if err := migrations.Up(dsn); err != nil {
		t.Fatalf("up from zero: %v", err)
	}
	if err := m.Up(); !errors.Is(err, migrate.ErrNoChange) {
		t.Fatalf("second up must be a no-op, got %v", err)
	}
	got, dirty, err := m.Version()
	if err != nil || dirty || got != all[len(all)-1] {
		t.Fatalf("final version %d dirty %v err %v", got, dirty, err)
	}

	q := db.New(pool)
	seed, err := importers.Seed(ctx, pool, q, "../../data/catalog.csv", "../../data/seeds/robot_specs.json")
	if err != nil {
		t.Fatalf("seed on a fresh schema: %v", err)
	}
	if seed.CatalogInserted == 0 {
		t.Fatal("seed inserted no catalog rows")
	}
	cat, err := catalogstore.Load(ctx, q)
	if err != nil || len(cat.Candidates) == 0 || cat.ContentSHA256 == "" {
		t.Fatalf("catalog on a fresh schema: %d rows, hash %q, %v", len(cat.Candidates), cat.ContentSHA256, err)
	}
	again, err := importers.Seed(ctx, pool, q, "../../data/catalog.csv", "../../data/seeds/robot_specs.json")
	if err != nil || again.CatalogInserted != 0 {
		t.Fatalf("second seed must not insert rows: %+v %v", again, err)
	}
}

func TestSolutionsFamilyBackfill(t *testing.T) {
	dsn := testdb.Empty(t)
	m, err := migrations.Open(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	if err := m.Migrate(23); err != nil {
		t.Fatalf("up to 23: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	cases := []struct {
		kind, typ, want string
	}{
		{"robot", "Мобильные роботы", "mobile"},
		{"bas", "БАС", "uav"},
		{"robot", " Мобильные манипуляторы ", "manipulator"},
		{"software", "ПО", "software"},
		{"robot", "Экзоскелеты", "other"},
		{"bas", "", "uav"},
		{"robot", "", ""},
	}
	for i, c := range cases {
		raw := `{}`
		if c.typ != "" {
			raw = `{"Тип": "` + c.typ + `"}`
		}
		if _, err := pool.Exec(ctx, `INSERT INTO solutions (id, name, kind, raw) VALUES ($1, $2, $3, $4)`,
			fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1), fmt.Sprintf("Робот %d", i+1), c.kind, raw); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Migrate(24); err != nil {
		t.Fatalf("up to 24: %v", err)
	}
	for i, c := range cases {
		var got *string
		if err := pool.QueryRow(ctx, `SELECT family FROM solutions WHERE id = $1`, fmt.Sprintf("00000000-0000-0000-0000-%012d", i+1)).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if (got == nil && c.want != "") || (got != nil && *got != c.want) {
			t.Errorf("%s %q: family %v, want %q", c.kind, c.typ, got, c.want)
		}
	}
}

func TestSeedKeepsEveryCatalogRowAsASolution(t *testing.T) {
	dsn := testdb.Empty(t)
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	const csv, specs = "../../data/catalog.csv", "../../data/seeds/robot_specs.json"
	seed, err := importers.Seed(ctx, pool, q, csv, specs)
	if err != nil {
		t.Fatal(err)
	}
	if seed.CatalogInserted != 223 || seed.SplitInserted != 0 {
		t.Fatalf("seed %+v: every one of the 223 rows is a solution", seed)
	}
	const h1500 = "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	count := func(where string, args ...any) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM solutions WHERE `+where, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(`raw->>'source_id' = $1 OR id::text = $1`, h1500); n != 2 {
		t.Fatalf("H1500 rows %d, want 2", n)
	}
	if n := count(`raw->>'source_id' IS NOT NULL`); n != 59 {
		t.Fatalf("rows of repeated ids %d, want 59 (23 ids, 36 extra rows)", n)
	}
	var withPayload int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM solutions s JOIN solution_specs sp ON sp.solution_id = s.id
		WHERE (s.id::text = $1 OR s.raw->>'source_id' = $1) AND sp.payload_kg = 1500`, h1500).Scan(&withPayload); err != nil {
		t.Fatal(err)
	}
	if withPayload != 2 {
		t.Fatalf("specs reach %d of the 2 H1500 rows", withPayload)
	}
	var multi int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM solutions WHERE jsonb_array_length(raw->'uses') > 1`).Scan(&multi); err != nil {
		t.Fatal(err)
	}
	if multi != 0 {
		t.Fatalf("%d rows still carry several uses", multi)
	}

	// A catalog seeded while repeated ids were merged gets its missing rows and a first row with its own use.
	if _, err := pool.Exec(ctx, `DELETE FROM solutions WHERE raw->>'source_id' IS NOT NULL AND id::text <> raw->>'source_id'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE solutions SET raw = (raw - 'source_id') || jsonb_build_object('uses', jsonb_build_array(raw->'uses'->0, raw->'uses'->0))
		WHERE id::text = $1`, h1500); err != nil {
		t.Fatal(err)
	}
	again, err := importers.Seed(ctx, pool, q, csv, specs)
	if err != nil {
		t.Fatal(err)
	}
	if again.CatalogInserted != 0 || again.SplitInserted != 36 {
		t.Fatalf("second seed %+v, want 36 rows added", again)
	}
	if n := count(`raw->>'source_id' IS NOT NULL`); n != 59 {
		t.Fatalf("rows of repeated ids %d after the second seed, want 59", n)
	}
	var uses int
	if err := pool.QueryRow(ctx, `SELECT jsonb_array_length(raw->'uses') FROM solutions WHERE id::text = $1`, h1500).Scan(&uses); err != nil {
		t.Fatal(err)
	}
	if uses != 1 {
		t.Fatalf("first row keeps %d uses", uses)
	}
	third, err := importers.Seed(ctx, pool, q, csv, specs)
	if err != nil || third.SplitInserted != 0 {
		t.Fatalf("third seed %+v %v", third, err)
	}
}

func TestSeedRecordsWhereEachValueComesFrom(t *testing.T) {
	dsn := testdb.Empty(t)
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	q := db.New(pool)
	const csv, specs = "../../data/catalog.csv", "../../data/seeds/robot_specs.json"
	if _, err := importers.Seed(ctx, pool, q, csv, specs); err != nil {
		t.Fatal(err)
	}
	const h1500 = "5760e938-9a43-45a7-b8e8-f4f2e6383930"
	source := func(id, code, key string) string {
		t.Helper()
		var v *string
		if err := pool.QueryRow(ctx, `SELECT field_sources->$2->>$3 FROM solutions WHERE id::text = $1`, id, code, key).Scan(&v); err != nil {
			t.Fatal(err)
		}
		if v == nil {
			return ""
		}
		return *v
	}
	if got := source(h1500, "payload_kg", "source_url"); got != "https://ronavi-robotics.ru/catalogue/h1500" {
		t.Fatalf("payload source %q", got)
	}
	if got := source(h1500, "min_aisle_mm", "confidence"); got != "vendor" {
		t.Fatalf("aisle confidence %q", got)
	}
	if got := source(h1500, "price_rub", "note"); got == "" {
		t.Fatal("the price carries no source")
	}
	if got := source(h1500, "lifetime_years", "source_url"); got != "" {
		t.Fatalf("a value nobody stated has a source %q", got)
	}
	var noted string
	if err := pool.QueryRow(ctx, `SELECT field_sources->'payload_kg'->>'note' FROM solutions WHERE name LIKE 'Ronavi SR%' LIMIT 1`).Scan(&noted); err != nil || noted == "" {
		t.Fatalf("a payload taken from the name must say so: %q %v", noted, err)
	}

	// A second seed keeps the entries; an admin's edit drops the entry of the value it changed and no other.
	if _, err := pool.Exec(ctx, `UPDATE solutions SET field_sources = jsonb_set(field_sources, '{payload_kg,note}', '"kept"') WHERE id::text = $1`, h1500); err != nil {
		t.Fatal(err)
	}
	if _, err := importers.Seed(ctx, pool, q, csv, specs); err != nil {
		t.Fatal(err)
	}
	if got := source(h1500, "payload_kg", "note"); got != "kept" {
		t.Fatalf("second seed rewrote a source: %q", got)
	}
	row, err := q.GetSolution(ctx, uuid.MustParse(h1500))
	if err != nil {
		t.Fatal(err)
	}
	listRow := db.ListSolutionsRow(row)
	after := importers.RowValues(listRow)
	after["payload_kg"] = 1400.0
	if err := importers.Save(ctx, q, listRow, after); err != nil {
		t.Fatal(err)
	}
	if got := source(h1500, "payload_kg", "source_url"); got != "" {
		t.Fatalf("an edited value keeps its old source %q", got)
	}
	if got := source(h1500, "width_mm", "source_url"); got == "" {
		t.Fatal("an edit of one value dropped the source of another")
	}
}

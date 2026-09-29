// Package testdb creates throwaway Postgres databases for integration tests.
package testdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"moscow_hackathon_2026/api/internal/db"
	"moscow_hackathon_2026/api/internal/importers"
	"moscow_hackathon_2026/api/migrations"
)

// Empty creates an empty database and returns its URL, or skips the test when TEST_DATABASE_URL is unset.
// The database is dropped when the test ends.
func Empty(t *testing.T) string {
	t.Helper()
	admin := os.Getenv("TEST_DATABASE_URL")
	if admin == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := pgxpool.New(ctx, admin)
	if err != nil {
		t.Fatalf("testdb.connect: %v", err)
	}
	name := "it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := root.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		root.Close()
		t.Fatalf("testdb.create: %v", err)
	}
	t.Cleanup(func() {
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		_, _ = root.Exec(dctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		root.Close()
	})
	u, err := url.Parse(admin)
	if err != nil {
		t.Fatalf("testdb.url: %v", err)
	}
	u.Path = "/" + name
	return u.String()
}

// New creates a migrated database with the seed catalog, or skips the test when TEST_DATABASE_URL is unset.
func New(t *testing.T, dataDir string) (*pgxpool.Pool, *db.Queries) {
	t.Helper()
	_, pool, q := NewDSN(t, dataDir)
	return pool, q
}

// NewDSN is New that also returns the database URL, for code that opens its own connection.
func NewDSN(t *testing.T, dataDir string) (string, *pgxpool.Pool, *db.Queries) {
	t.Helper()
	dsn := Empty(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := migrateUp(dsn); err != nil {
		t.Fatalf("testdb.migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("testdb.open: %v", err)
	}
	t.Cleanup(pool.Close)
	q := db.New(pool)
	if _, err := importers.Seed(ctx, pool, q, dataDir+"/catalog.csv", dataDir+"/seeds/robot_specs.json"); err != nil {
		t.Fatalf("testdb.seed: %v", err)
	}
	return dsn, pool, q
}

// migrateUp applies all migrations, then rolls back and reapplies the latest one.
func migrateUp(dsn string) error {
	m, err := migrations.Open(dsn)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("up: %w", err)
	}
	if err := m.Steps(-1); err != nil {
		return fmt.Errorf("down latest: %w", err)
	}
	if err := m.Up(); err != nil {
		return fmt.Errorf("up again: %w", err)
	}
	return nil
}

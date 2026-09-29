package migrations

import (
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var FS embed.FS

// Open returns a migrator for a postgres:// or postgresql:// URL. The caller closes it.
func Open(databaseURL string) (*migrate.Migrate, error) {
	src, err := iofs.New(FS, ".")
	if err != nil {
		return nil, fmt.Errorf("migrate.source: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, driverURL(databaseURL))
	if err != nil {
		return nil, fmt.Errorf("migrate.open: %w", err)
	}
	return m, nil
}

// Up applies every pending migration.
func Up(databaseURL string) error {
	m, err := Open(databaseURL)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate.up: %w", err)
	}
	return nil
}

func driverURL(databaseURL string) string {
	u := strings.Replace(databaseURL, "postgres://", "pgx5://", 1)
	return strings.Replace(u, "postgresql://", "pgx5://", 1)
}

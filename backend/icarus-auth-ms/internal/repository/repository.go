package repository

import (
	"database/sql"
	"embed"
	"errors"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/lib/pq"
	_ "modernc.org/sqlite"
)

//go:embed migrations
var migrationsFS embed.FS

type SQLRepository struct {
	db     *sql.DB
	driver string
}

func NewSQLRepository(db *sql.DB, driver string) *SQLRepository {
	return &SQLRepository{db: db, driver: driver}
}

// InitDB initializes the database (SQLite or PostgreSQL) and runs migration queries.
func InitDB(driver, connStr string) (*sql.DB, error) {
	db, err := sql.Open(driver, connStr)
	if err != nil {
		return nil, err
	}

	if err = db.Ping(); err != nil {
		db.Close()
		return nil, err
	}

	if driver == "sqlite" {
		_, _ = db.Exec("PRAGMA foreign_keys = ON;")
	}

	if driver == "postgres" {
		d, err := iofs.New(migrationsFS, "migrations/postgres")
		if err != nil {
			db.Close()
			return nil, err
		}

		driverInstance, err := postgres.WithInstance(db, &postgres.Config{})
		if err != nil {
			db.Close()
			return nil, err
		}

		m, err := migrate.NewWithInstance("iofs", d, "postgres", driverInstance)
		if err != nil {
			db.Close()
			return nil, err
		}

		if err := m.Up(); err != nil && err != migrate.ErrNoChange {
			db.Close()
			return nil, err
		}
	} else {
		// SQLite manual migration parsing to preserve CGO-free build.
		// Uses a schema_migrations table to track which files have already been
		// applied, so non-idempotent statements (e.g. ALTER TABLE ADD COLUMN)
		// are never executed more than once.
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`); err != nil {
			db.Close()
			return nil, err
		}

		entries, err := migrationsFS.ReadDir("migrations/sqlite")
		if err != nil {
			db.Close()
			return nil, err
		}

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".up.sql") {
				continue
			}

			version := entry.Name()

			// Skip migrations that have already been applied.
			var count int
			if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&count); err != nil {
				db.Close()
				return nil, err
			}
			if count > 0 {
				continue
			}

			upSQL, err := migrationsFS.ReadFile("migrations/sqlite/" + version)
			if err != nil {
				db.Close()
				return nil, err
			}

			statements := strings.Split(string(upSQL), ";")
			for _, stmt := range statements {
				trimmed := strings.TrimSpace(stmt)
				if trimmed == "" {
					continue
				}
				if _, err := db.Exec(trimmed); err != nil {
					db.Close()
					return nil, err
				}
			}

			// Record the migration as applied.
			if _, err := db.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, version); err != nil {
				db.Close()
				return nil, err
			}
		}
	}

	return db, nil
}

// Helper function to parse SQLite datetime formats.
func parseTime(timeStr string) (time.Time, error) {
	layouts := []string{
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
	}

	for _, layout := range layouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("failed to parse time: " + timeStr)
}

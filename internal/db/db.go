package db

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(dbPath string, maxOpenConns, maxIdleConns int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent read/write performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Wait up to 5s for locks instead of failing immediately with SQLITE_BUSY
	db.Exec("PRAGMA busy_timeout = 5000")
	db.Exec("PRAGMA foreign_keys = ON")

	// Connection pool tuning for SQLite
	db.SetMaxOpenConns(maxOpenConns)
	db.SetMaxIdleConns(maxIdleConns)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Execute the embedded schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	// Apply incremental column additions for existing databases. SQLite has no
	// "ADD COLUMN IF NOT EXISTS", so we attempt the ALTER and ignore the error
	// when the column already exists (duplicate column error).
	if err := migrateSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to migrate schema: %w", err)
	}

	return db, nil
}

// migrateSchema applies additive schema changes that cannot be expressed as
// idempotent DDL in schema.sql (e.g. SQLite ADD COLUMN has no IF NOT EXISTS).
func migrateSchema(db *sql.DB) error {
	alterations := []string{
		"ALTER TABLE sites ADD COLUMN ssl_expiry DATETIME",
		// Safe migration: add role column if it doesn't exist (no-op on fresh DBs)
		"ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'",
	}
	for _, stmt := range alterations {
		if _, err := db.Exec(stmt); err != nil {
			// SQLite returns "duplicate column name" when the column already
			// exists. Treat that as a no-op; any other error is fatal.
			if !isDuplicateColumnError(err) {
				return fmt.Errorf("migration failed (%s): %w", stmt, err)
			}
		}
	}
	return nil
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "duplicate column name")
}

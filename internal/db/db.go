package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

func Open(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Execute the embedded schema
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to execute schema: %w", err)
	}

	// Add columns to existing tables if they don't exist yet (safe for both fresh and existing DBs)
	migrations := []string{
		"ALTER TABLE sites ADD COLUMN is_local INTEGER DEFAULT 0",
		"ALTER TABLE sites ADD COLUMN compose_path TEXT",
		"ALTER TABLE sites ADD COLUMN routing_config TEXT",
		"ALTER TABLE servers ADD COLUMN ssh_host_key TEXT",
	}
	for _, m := range migrations {
		_, _ = db.Exec(m) // Ignore "duplicate column" errors
	}

	return db, nil
}

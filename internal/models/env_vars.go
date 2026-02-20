package models

import (
	"database/sql"
	"fmt"
)

type EnvVar struct {
	ID        int
	SiteID    int
	Key       string
	Value     string
	CreatedAt string
}

func GetEnvVarsBySiteID(db *sql.DB, siteID int) ([]EnvVar, error) {
	rows, err := db.Query(
		"SELECT id, site_id, key, value, created_at FROM site_env_vars WHERE site_id = ? ORDER BY key ASC",
		siteID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query env vars: %w", err)
	}
	defer rows.Close()

	var vars []EnvVar
	for rows.Next() {
		var v EnvVar
		if err := rows.Scan(&v.ID, &v.SiteID, &v.Key, &v.Value, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan env var: %w", err)
		}
		vars = append(vars, v)
	}
	return vars, rows.Err()
}

func CreateEnvVar(db *sql.DB, siteID int, key, value string) error {
	_, err := db.Exec(
		"INSERT INTO site_env_vars (site_id, key, value) VALUES (?, ?, ?) ON CONFLICT(site_id, key) DO UPDATE SET value = excluded.value",
		siteID, key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to create env var: %w", err)
	}
	return nil
}

func DeleteEnvVar(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM site_env_vars WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete env var: %w", err)
	}
	return nil
}

// RenderEnvFile generates a .env file string from the site's env vars.
func RenderEnvFile(db *sql.DB, siteID int) (string, error) {
	vars, err := GetEnvVarsBySiteID(db, siteID)
	if err != nil {
		return "", err
	}
	if len(vars) == 0 {
		return "", nil
	}
	var content string
	for _, v := range vars {
		content += v.Key + "=" + v.Value + "\n"
	}
	return content, nil
}

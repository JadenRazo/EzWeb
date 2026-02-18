package models

import (
	"database/sql"
	"fmt"
	"time"
)

type SiteTemplate struct {
	ID          int
	Slug        string
	Label       string
	Description string
	CreatedAt   time.Time
}

func GetAllTemplates(db *sql.DB) ([]SiteTemplate, error) {
	rows, err := db.Query(
		"SELECT id, slug, label, COALESCE(description,''), created_at FROM site_templates ORDER BY label ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query templates: %w", err)
	}
	defer rows.Close()

	var templates []SiteTemplate
	for rows.Next() {
		var t SiteTemplate
		if err := rows.Scan(&t.ID, &t.Slug, &t.Label, &t.Description, &t.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan template row: %w", err)
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

func GetTemplateBySlug(db *sql.DB, slug string) (*SiteTemplate, error) {
	t := &SiteTemplate{}
	err := db.QueryRow(
		"SELECT id, slug, label, COALESCE(description,''), created_at FROM site_templates WHERE slug = ?",
		slug,
	).Scan(&t.ID, &t.Slug, &t.Label, &t.Description, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("template not found: %w", err)
	}
	return t, nil
}

package models

import (
	"database/sql"
	"fmt"
)

type HealthCheck struct {
	ID              int
	SiteID          int
	HTTPStatus      int
	LatencyMs       int
	ContainerStatus string
	CheckedAt       string
}

func CreateHealthCheck(db *sql.DB, h *HealthCheck) error {
	result, err := db.Exec(
		`INSERT INTO health_checks (site_id, http_status, latency_ms, container_status)
		 VALUES (?, ?, ?, ?)`,
		h.SiteID, h.HTTPStatus, h.LatencyMs, h.ContainerStatus,
	)
	if err != nil {
		return fmt.Errorf("failed to create health check: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	h.ID = int(id)
	return nil
}

func GetHealthChecksBySiteID(db *sql.DB, siteID int, limit int) ([]HealthCheck, error) {
	rows, err := db.Query(
		`SELECT id, site_id, COALESCE(http_status,0), COALESCE(latency_ms,0),
		        COALESCE(container_status,''), checked_at
		 FROM health_checks
		 WHERE site_id = ?
		 ORDER BY checked_at DESC
		 LIMIT ?`,
		siteID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query health checks: %w", err)
	}
	defer rows.Close()

	var checks []HealthCheck
	for rows.Next() {
		var hc HealthCheck
		if err := rows.Scan(&hc.ID, &hc.SiteID, &hc.HTTPStatus, &hc.LatencyMs, &hc.ContainerStatus, &hc.CheckedAt); err != nil {
			return nil, fmt.Errorf("failed to scan health check row: %w", err)
		}
		checks = append(checks, hc)
	}
	return checks, rows.Err()
}

func GetLatestHealthCheck(db *sql.DB, siteID int) (*HealthCheck, error) {
	hc := &HealthCheck{}
	err := db.QueryRow(
		`SELECT id, site_id, COALESCE(http_status,0), COALESCE(latency_ms,0),
		        COALESCE(container_status,''), checked_at
		 FROM health_checks
		 WHERE site_id = ?
		 ORDER BY checked_at DESC
		 LIMIT 1`,
		siteID,
	).Scan(&hc.ID, &hc.SiteID, &hc.HTTPStatus, &hc.LatencyMs, &hc.ContainerStatus, &hc.CheckedAt)
	if err != nil {
		return nil, fmt.Errorf("health check not found: %w", err)
	}
	return hc, nil
}

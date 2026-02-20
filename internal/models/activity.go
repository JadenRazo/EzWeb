package models

import (
	"database/sql"
	"fmt"
	"log"
)

type Activity struct {
	ID         int
	EntityType string
	EntityID   int
	Action     string
	Details    string
	IPAddress  string
	UserAgent  string
	CreatedAt  string
}

func LogActivity(db *sql.DB, entityType string, entityID int, action string, details string) {
	if _, err := db.Exec(
		"INSERT INTO activity_log (entity_type, entity_id, action, details) VALUES (?, ?, ?, ?)",
		entityType, entityID, action, details,
	); err != nil {
		log.Printf("failed to log activity (%s/%d %s): %v", entityType, entityID, action, err)
	}
}

// LogActivityWithContext logs an activity with the request's IP and user agent.
func LogActivityWithContext(db *sql.DB, entityType string, entityID int, action string, details string, ipAddress string, userAgent string) {
	if _, err := db.Exec(
		"INSERT INTO activity_log (entity_type, entity_id, action, details, ip_address, user_agent) VALUES (?, ?, ?, ?, ?, ?)",
		entityType, entityID, action, details, ipAddress, userAgent,
	); err != nil {
		log.Printf("failed to log activity (%s/%d %s): %v", entityType, entityID, action, err)
	}
}

// LogActivityAt inserts an activity with a specific timestamp (for backfilling).
func LogActivityAt(db *sql.DB, entityType string, entityID int, action string, details string, createdAt string) {
	if _, err := db.Exec(
		"INSERT INTO activity_log (entity_type, entity_id, action, details, created_at) VALUES (?, ?, ?, ?, ?)",
		entityType, entityID, action, details, createdAt,
	); err != nil {
		log.Printf("failed to log activity at %s (%s/%d %s): %v", createdAt, entityType, entityID, action, err)
	}
}

func GetRecentActivities(db *sql.DB, limit int) ([]Activity, error) {
	rows, err := db.Query(
		"SELECT id, entity_type, COALESCE(entity_id,0), action, COALESCE(details,''), COALESCE(ip_address,''), COALESCE(user_agent,''), created_at FROM activity_log ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query activities: %w", err)
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.EntityType, &a.EntityID, &a.Action, &a.Details, &a.IPAddress, &a.UserAgent, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

// BackfillActivities seeds the activity_log with entries for existing entities
// that have no activity recorded yet. Safe to call on every startup.
func BackfillActivities(db *sql.DB) {
	var count int
	_ = db.QueryRow("SELECT COUNT(*) FROM activity_log").Scan(&count)
	if count > 0 {
		return // already has data, skip backfill
	}

	log.Println("Backfilling activity log for existing entities...")

	// Backfill sites
	rows, err := db.Query("SELECT id, domain, status, created_at FROM sites ORDER BY created_at ASC")
	if err == nil {
		for rows.Next() {
			var id int
			var domain, status, createdAt string
			if err := rows.Scan(&id, &domain, &status, &createdAt); err == nil {
				LogActivityAt(db, "site", id, "created", "Site "+domain+" added", createdAt)
				if status == "running" {
					LogActivityAt(db, "site", id, "deployed", "Site "+domain+" deployed", createdAt)
				}
			}
		}
		rows.Close()
	}

	// Backfill servers
	sRows, err := db.Query("SELECT id, name, created_at FROM servers ORDER BY created_at ASC")
	if err == nil {
		for sRows.Next() {
			var id int
			var name, createdAt string
			if err := sRows.Scan(&id, &name, &createdAt); err == nil {
				LogActivityAt(db, "server", id, "created", "Added server "+name, createdAt)
			}
		}
		sRows.Close()
	}

	// Backfill customers
	cRows, err := db.Query("SELECT id, name, created_at FROM customers ORDER BY created_at ASC")
	if err == nil {
		for cRows.Next() {
			var id int
			var name, createdAt string
			if err := cRows.Scan(&id, &name, &createdAt); err == nil {
				LogActivityAt(db, "customer", id, "created", "Customer "+name+" added", createdAt)
			}
		}
		cRows.Close()
	}

	log.Println("Activity log backfill complete")
}

package models

import (
	"database/sql"
	"fmt"
)

type Activity struct {
	ID         int
	EntityType string
	EntityID   int
	Action     string
	Details    string
	CreatedAt  string
}

func LogActivity(db *sql.DB, entityType string, entityID int, action string, details string) {
	_, _ = db.Exec(
		"INSERT INTO activity_log (entity_type, entity_id, action, details) VALUES (?, ?, ?, ?)",
		entityType, entityID, action, details,
	)
}

func GetRecentActivities(db *sql.DB, limit int) ([]Activity, error) {
	rows, err := db.Query(
		"SELECT id, entity_type, COALESCE(entity_id,0), action, COALESCE(details,''), created_at FROM activity_log ORDER BY created_at DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query activities: %w", err)
	}
	defer rows.Close()

	var activities []Activity
	for rows.Next() {
		var a Activity
		if err := rows.Scan(&a.ID, &a.EntityType, &a.EntityID, &a.Action, &a.Details, &a.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan activity: %w", err)
		}
		activities = append(activities, a)
	}
	return activities, rows.Err()
}

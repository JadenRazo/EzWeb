package handlers

import (
	"database/sql"
	"log"
	"strconv"

	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func Dashboard(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var customerCount, siteCount, serverCount, overdueCount int
		var runningCount, stoppedCount, errorCount int
		var serversOnline, serversOffline, serversUnknown int

		// Aggregate site counts in a single query.
		if err := db.QueryRow(`SELECT
			COUNT(*),
			SUM(CASE WHEN status = 'running' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'stopped' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END)
			FROM sites`).Scan(&siteCount, &runningCount, &stoppedCount, &errorCount); err != nil {
			log.Printf("dashboard sites query failed: %v", err)
		}

		// Aggregate server counts in a single query.
		if err := db.QueryRow(`SELECT
			COUNT(*),
			SUM(CASE WHEN status IN ('online','active') THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'offline' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status NOT IN ('online','active','offline') THEN 1 ELSE 0 END)
			FROM servers`).Scan(&serverCount, &serversOnline, &serversOffline, &serversUnknown); err != nil {
			log.Printf("dashboard servers query failed: %v", err)
		}

		// Customer and overdue payment counts.
		if err := db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&customerCount); err != nil {
			log.Printf("dashboard customer count failed: %v", err)
		}
		if err := db.QueryRow("SELECT COUNT(*) FROM payments WHERE paid_at IS NULL AND due_date < date('now')").Scan(&overdueCount); err != nil {
			log.Printf("dashboard overdue count failed: %v", err)
		}

		activities, err := models.GetRecentActivities(db, 10)
		if err != nil {
			log.Printf("dashboard: failed to load activities: %v", err)
		}

		data := pages.DashboardData{
			CustomerCount:  strconv.Itoa(customerCount),
			SiteCount:      strconv.Itoa(siteCount),
			ServerCount:    strconv.Itoa(serverCount),
			OverdueCount:   strconv.Itoa(overdueCount),
			RunningCount:   runningCount,
			StoppedCount:   stoppedCount,
			ErrorCount:     errorCount,
			ServersOnline:  serversOnline,
			ServersOffline: serversOffline,
			ServersUnknown: serversUnknown,
			Activities:     activities,
		}

		c.Set("Content-Type", "text/html")
		return pages.Dashboard(data).Render(c.Context(), c.Response().BodyWriter())
	}
}

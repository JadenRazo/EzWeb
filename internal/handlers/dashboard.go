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

		scanCount := func(query string, dest *int) {
			if err := db.QueryRow(query).Scan(dest); err != nil {
				log.Printf("dashboard query failed (%s): %v", query, err)
			}
		}

		scanCount("SELECT COUNT(*) FROM customers", &customerCount)
		scanCount("SELECT COUNT(*) FROM sites", &siteCount)
		scanCount("SELECT COUNT(*) FROM servers", &serverCount)
		scanCount("SELECT COUNT(*) FROM payments WHERE paid_at IS NULL AND due_date < date('now')", &overdueCount)
		scanCount("SELECT COUNT(*) FROM sites WHERE status = 'running'", &runningCount)
		scanCount("SELECT COUNT(*) FROM sites WHERE status = 'stopped'", &stoppedCount)
		scanCount("SELECT COUNT(*) FROM sites WHERE status = 'error'", &errorCount)
		scanCount("SELECT COUNT(*) FROM servers WHERE status IN ('online','active')", &serversOnline)
		scanCount("SELECT COUNT(*) FROM servers WHERE status = 'offline'", &serversOffline)
		scanCount("SELECT COUNT(*) FROM servers WHERE status NOT IN ('online','active','offline')", &serversUnknown)

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

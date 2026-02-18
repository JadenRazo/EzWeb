package handlers

import (
	"database/sql"
	"strconv"

	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func Dashboard(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var customerCount, siteCount, serverCount, overdueCount int
		var runningCount, stoppedCount, errorCount int

		_ = db.QueryRow("SELECT COUNT(*) FROM customers").Scan(&customerCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM sites").Scan(&siteCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM servers").Scan(&serverCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM payments WHERE paid_at IS NULL AND due_date < date('now')").Scan(&overdueCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM sites WHERE status = 'running'").Scan(&runningCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM sites WHERE status = 'stopped'").Scan(&stoppedCount)
		_ = db.QueryRow("SELECT COUNT(*) FROM sites WHERE status = 'error'").Scan(&errorCount)

		activities, _ := models.GetRecentActivities(db, 10)

		data := pages.DashboardData{
			CustomerCount: strconv.Itoa(customerCount),
			SiteCount:     strconv.Itoa(siteCount),
			ServerCount:   strconv.Itoa(serverCount),
			OverdueCount:  strconv.Itoa(overdueCount),
			RunningCount:  runningCount,
			StoppedCount:  stoppedCount,
			ErrorCount:    errorCount,
			Activities:    activities,
		}

		c.Set("Content-Type", "text/html")
		return pages.Dashboard(data).Render(c.Context(), c.Response().BodyWriter())
	}
}

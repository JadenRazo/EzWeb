package mcptools

import (
	"database/sql"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func RegisterTools(s *server.MCPServer, db *sql.DB) {
	h := &handlers{db: db}

	s.AddTool(
		mcp.NewTool("list_sites",
			mcp.WithDescription("List all managed sites with their status, domain, server, and template info. Optionally filter by status or server."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("status", mcp.Description("Filter by site status (running, stopped, error, pending, deploying)")),
			mcp.WithNumber("server_id", mcp.Description("Filter by server ID")),
		),
		h.listSites,
	)

	s.AddTool(
		mcp.NewTool("get_site_logs",
			mcp.WithDescription("Fetch recent Docker container logs for a site. Provide either site_id or domain to identify the site."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("site_id", mcp.Description("Site ID")),
			mcp.WithString("domain", mcp.Description("Site domain name")),
			mcp.WithNumber("tail", mcp.Description("Number of log lines to return (default 100)")),
		),
		h.getSiteLogs,
	)

	s.AddTool(
		mcp.NewTool("get_site_health",
			mcp.WithDescription("Get health check history for a site including HTTP status, latency, and container status."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("site_id", mcp.Description("Site ID")),
			mcp.WithString("domain", mcp.Description("Site domain name")),
			mcp.WithNumber("limit", mcp.Description("Number of health checks to return (default 10)")),
		),
		h.getSiteHealth,
	)

	s.AddTool(
		mcp.NewTool("get_site_errors",
			mcp.WithDescription("Find sites with problems: error/stopped status or recent failed health checks."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("hours", mcp.Description("Look back period in hours (default 24)")),
		),
		h.getSiteErrors,
	)

	s.AddTool(
		mcp.NewTool("get_activity_log",
			mcp.WithDescription("Get recent activity feed showing site deployments, status changes, and other events."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("limit", mcp.Description("Number of activities to return (default 20)")),
			mcp.WithString("entity_type", mcp.Description("Filter by entity type (site, server, customer, payment)")),
		),
		h.getActivityLog,
	)

	s.AddTool(
		mcp.NewTool("list_servers",
			mcp.WithDescription("List all configured servers with their connection details and status."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		h.listServers,
	)

	s.AddTool(
		mcp.NewTool("get_server_status",
			mcp.WithDescription("Test live SSH connectivity and Docker availability on a server."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithNumber("server_id", mcp.Description("Server ID to check"), mcp.Required()),
		),
		h.getServerStatus,
	)

	s.AddTool(
		mcp.NewTool("backup_database",
			mcp.WithDescription("Create a backup of the EzWeb SQLite database. Returns the path to the backup file."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithString("path", mcp.Description("Backup file path (default: ./ezweb.db.bak)")),
		),
		h.backupDatabase,
	)
}

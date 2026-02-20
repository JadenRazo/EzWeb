package handlers

import (
	"context"
	"database/sql"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ezweb/internal/caddy"
	"ezweb/internal/docker"
	"ezweb/internal/models"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

var domainRegex = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?\.)*[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

func validateDomain(domain string) bool {
	if len(domain) == 0 || len(domain) > 253 {
		return false
	}
	return domainRegex.MatchString(domain)
}

func validatePort(port int) bool {
	return port >= 1024 && port <= 65535
}

func ListSites(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}

		searchQuery := strings.TrimSpace(c.Query("q", ""))
		statusFilter := strings.TrimSpace(c.Query("status", ""))

		sites, total, err := models.SearchSites(db, searchQuery, statusFilter, page, perPage)
		if err != nil {
			log.Printf("failed to list sites: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load sites")
		}

		servers, err := models.GetAllServers(db)
		if err != nil {
			log.Printf("failed to load servers for site form: %v", err)
		}

		templates, err := models.GetAllTemplates(db)
		if err != nil {
			log.Printf("failed to load templates for site form: %v", err)
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers for site form: %v", err)
		}

		c.Set("Content-Type", "text/html")
		return pages.Sites(sites, servers, templates, customers, page, total, perPage, searchQuery, statusFilter).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateSiteForm(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		servers, err := models.GetAllServers(db)
		if err != nil {
			log.Printf("failed to load servers: %v", err)
		}

		templates, err := models.GetAllTemplates(db)
		if err != nil {
			log.Printf("failed to load templates: %v", err)
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers: %v", err)
		}

		c.Set("Content-Type", "text/html")
		return pages.SiteForm(servers, templates, customers).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateSite(db *sql.DB, caddyMgr *caddy.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		domain := strings.TrimSpace(c.FormValue("domain"))
		if domain == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Domain is required")
		}
		if !validateDomain(domain) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid domain format")
		}

		templateSlug := c.FormValue("template_slug")
		composePath := strings.TrimSpace(c.FormValue("compose_path"))
		isLocal := c.FormValue("is_local") == "1" || c.FormValue("is_local") == "on"

		// Template is required only for non-imported sites (no compose_path)
		if templateSlug == "" && composePath == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Template or compose path is required")
		}

		containerName := c.FormValue("container_name")
		if containerName == "" {
			containerName = strings.ReplaceAll(domain, ".", "-")
		}

		if err := docker.ValidateContainerName(containerName); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid container name: " + err.Error())
		}

		port, err := strconv.Atoi(c.FormValue("port", "0"))
		if err != nil || port == 0 {
			port, err = nextAvailablePort(db)
			if err != nil {
				log.Printf("failed to assign port: %v", err)
				port = 8080
			}
		}
		if !validatePort(port) {
			return c.Status(fiber.StatusBadRequest).SendString("Port must be between 1024 and 65535")
		}

		var serverID sql.NullInt64
		if sid := c.FormValue("server_id"); sid != "" {
			if v, err := strconv.ParseInt(sid, 10, 64); err == nil {
				serverID = sql.NullInt64{Int64: v, Valid: true}
			}
		}

		var customerID sql.NullInt64
		if cid := c.FormValue("customer_id"); cid != "" {
			if v, err := strconv.ParseInt(cid, 10, 64); err == nil {
				customerID = sql.NullInt64{Int64: v, Valid: true}
			}
		}

		site := &models.Site{
			Domain:        domain,
			ServerID:      serverID,
			TemplateSlug:  templateSlug,
			CustomerID:    customerID,
			ContainerName: containerName,
			Port:          port,
			Status:        "pending",
			SSLEnabled:    false,
			IsLocal:       isLocal,
			ComposePath:   composePath,
		}

		if err := models.CreateSite(db, site); err != nil {
			log.Printf("failed to create site: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create site")
		}

		// Trigger Caddy reload
		if caddyMgr != nil {
			if err := caddyMgr.AddSite(db, *site); err != nil {
				log.Printf("caddy reload failed after creating site %s: %v", domain, err)
			}
		}

		models.LogActivityWithContext(db, "site", site.ID, "created", "Created site "+site.Domain, c.IP(), c.Get("User-Agent"))

		created, err := models.GetSiteByID(db, site.ID)
		if err != nil {
			log.Printf("failed to reload created site: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Site created but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*created).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites")
	}
}

func SiteDetail(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		servers, _ := models.GetAllServers(db)
		templates, _ := models.GetAllTemplates(db)
		customers, _ := models.GetAllCustomers(db)

		c.Set("Content-Type", "text/html")
		return pages.SiteDetailPage(*site, servers, templates, customers).Render(c.Context(), c.Response().BodyWriter())
	}
}

func DeploySite(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := docker.LocalComposeUp(ctx, site.ComposePath); err != nil {
				log.Printf("local deploy failed for site %d (%s): %v", id, site.Domain, err)
				_ = models.UpdateSiteStatus(db, id, "error")
				return c.Status(fiber.StatusInternalServerError).SendString("Deploy failed")
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("No server assigned to this site")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Assigned server not found")
			}

			envContent, _ := models.RenderEnvFile(db, id)
			if err := docker.DeploySite(
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey,
				site.Domain, site.TemplateSlug, site.ContainerName, site.Port, envContent,
			); err != nil {
				log.Printf("deploy failed for site %d (%s): %v", id, site.Domain, err)
				_ = models.UpdateSiteStatus(db, id, "error")
				return c.Status(fiber.StatusInternalServerError).SendString("Deployment failed")
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivityWithContext(db, "site", id, "deployed", "Deployed site "+site.Domain, c.IP(), c.Get("User-Agent"))

		site, _ = models.GetSiteByID(db, id)
		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*site).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites/" + strconv.Itoa(id))
	}
}

func StartSite(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := docker.LocalComposeStart(ctx, site.ComposePath); err != nil {
				log.Printf("local start failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Start failed")
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("No server assigned")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Server not found")
			}

			if err := docker.StartSiteRemote(
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName,
			); err != nil {
				log.Printf("start failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Start failed")
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivityWithContext(db, "site", id, "started", "Started site "+site.Domain, c.IP(), c.Get("User-Agent"))

		site, _ = models.GetSiteByID(db, id)
		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*site).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites/" + strconv.Itoa(id))
	}
}

func StopSite(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := docker.LocalComposeStop(ctx, site.ComposePath); err != nil {
				log.Printf("local stop failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Stop failed")
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("No server assigned")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Server not found")
			}

			if err := docker.StopSiteRemote(
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName,
			); err != nil {
				log.Printf("stop failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Stop failed")
			}
		}

		_ = models.UpdateSiteStatus(db, id, "stopped")
		models.LogActivityWithContext(db, "site", id, "stopped", "Stopped site "+site.Domain, c.IP(), c.Get("User-Agent"))

		site, _ = models.GetSiteByID(db, id)
		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*site).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites/" + strconv.Itoa(id))
	}
}

func RestartSite(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if err := docker.LocalComposeRestart(ctx, site.ComposePath); err != nil {
				log.Printf("local restart failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Restart failed")
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("No server assigned")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Server not found")
			}

			if err := docker.RestartSiteRemote(
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName,
			); err != nil {
				log.Printf("restart failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Restart failed")
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivityWithContext(db, "site", id, "restarted", "Restarted site "+site.Domain, c.IP(), c.Get("User-Agent"))

		site, _ = models.GetSiteByID(db, id)
		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*site).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites/" + strconv.Itoa(id))
	}
}

func DeleteSite(db *sql.DB, caddyMgr *caddy.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		// Attempt remote cleanup if a server is assigned (non-local)
		if !site.IsLocal && site.ServerID.Valid {
			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err == nil {
				if rmErr := docker.RemoveSiteRemote(
					server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName,
				); rmErr != nil {
					log.Printf("remote cleanup failed for site %d: %v (continuing with DB delete)", id, rmErr)
				}
			}
		}

		domain := site.Domain

		// Remove health check history before deleting the site — health checks
		// are meaningless without their associated site.
		if _, err := db.Exec("DELETE FROM health_checks WHERE site_id = ?", id); err != nil {
			log.Printf("failed to remove health checks for site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to remove site health checks")
		}

		if err := models.DeleteSite(db, id); err != nil {
			log.Printf("failed to delete site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete site")
		}
		models.LogActivityWithContext(db, "site", id, "deleted", "Deleted site "+domain, c.IP(), c.Get("User-Agent"))

		// Trigger Caddy reload
		if caddyMgr != nil {
			if err := caddyMgr.RemoveSite(db, domain); err != nil {
				log.Printf("caddy reload failed after deleting site %s: %v", domain, err)
			}
		}

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/sites")
	}
}

func UpdateSite(db *sql.DB, caddyMgr *caddy.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		existing, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		domain := strings.TrimSpace(c.FormValue("domain"))
		if domain == "" {
			domain = existing.Domain
		}

		templateSlug := c.FormValue("template_slug")
		if templateSlug == "" {
			templateSlug = existing.TemplateSlug
		}

		containerName := c.FormValue("container_name")
		if containerName == "" {
			containerName = existing.ContainerName
		}

		composePath := strings.TrimSpace(c.FormValue("compose_path"))
		if composePath == "" {
			composePath = existing.ComposePath
		}

		port, err := strconv.Atoi(c.FormValue("port", strconv.Itoa(existing.Port)))
		if err != nil {
			port = existing.Port
		}

		var serverID sql.NullInt64
		if sid := c.FormValue("server_id"); sid != "" {
			if v, err := strconv.ParseInt(sid, 10, 64); err == nil {
				serverID = sql.NullInt64{Int64: v, Valid: true}
			}
		} else {
			serverID = existing.ServerID
		}

		var customerID sql.NullInt64
		if cid := c.FormValue("customer_id"); cid != "" {
			if v, err := strconv.ParseInt(cid, 10, 64); err == nil {
				customerID = sql.NullInt64{Int64: v, Valid: true}
			}
		} else {
			customerID = existing.CustomerID
		}

		isLocal := existing.IsLocal
		if c.FormValue("is_local") != "" {
			isLocal = c.FormValue("is_local") == "1" || c.FormValue("is_local") == "on"
		}

		site := &models.Site{
			ID:            id,
			Domain:        domain,
			ServerID:      serverID,
			TemplateSlug:  templateSlug,
			CustomerID:    customerID,
			ContainerName: containerName,
			Port:          port,
			Status:        existing.Status,
			SSLEnabled:    existing.SSLEnabled,
			IsLocal:       isLocal,
			ComposePath:   composePath,
			RoutingConfig: existing.RoutingConfig,
		}

		if err := models.UpdateSite(db, site); err != nil {
			log.Printf("failed to update site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update site")
		}

		models.LogActivityWithContext(db, "site", id, "updated", "Updated site "+domain, c.IP(), c.Get("User-Agent"))

		// Trigger Caddy reload if domain, port, or routing changed
		needsReload := domain != existing.Domain || port != existing.Port
		if caddyMgr != nil && needsReload {
			if err := caddyMgr.AddSite(db, *site); err != nil {
				log.Printf("caddy reload failed after updating site %s: %v", domain, err)
			}
		}

		updated, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Site updated but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SiteRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/sites")
	}
}

// nextAvailablePort returns the next unused port starting from 8080.
// The SELECT runs inside a transaction so concurrent site creations read a
// consistent snapshot of the current maximum, reducing (though not eliminating)
// the window for a port collision. The UNIQUE index on sites(port) acts as the
// final guard — the INSERT will fail fast on a true collision and the caller
// can surface an appropriate error.
func nextAvailablePort(db *sql.DB) (int, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	var maxPort sql.NullInt64
	if err := tx.QueryRow("SELECT MAX(port) FROM sites").Scan(&maxPort); err != nil {
		return 0, err
	}

	port := 8080
	if maxPort.Valid && maxPort.Int64 >= 8080 {
		port = int(maxPort.Int64) + 1
	}

	// Commit the read-only transaction; the actual write happens in CreateSite.
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return port, nil
}

// --- Environment Variable Handlers ---

func ListSiteEnvVars(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		vars, err := models.GetEnvVarsBySiteID(db, id)
		if err != nil {
			log.Printf("failed to get env vars for site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load environment variables")
		}

		if len(vars) == 0 {
			return c.SendString("<p class='text-sm text-gray-400'>No environment variables set.</p>")
		}

		out := "<div class='space-y-2'>"
		for _, v := range vars {
			out += "<div class='flex items-center justify-between p-2 bg-gray-50 rounded-lg'>"
			out += "<div class='font-mono text-sm'><span class='font-semibold text-gray-700'>" + html.EscapeString(v.Key) + "</span> = <span class='text-gray-500'>" + html.EscapeString(v.Value) + "</span></div>"
			out += "<button hx-delete='/sites/" + strconv.Itoa(id) + "/env/" + strconv.Itoa(v.ID) + "' hx-target='#env-list' hx-swap='innerHTML' hx-confirm='Delete this variable?' "
			out += "class='px-2 py-1 text-xs text-red-600 hover:bg-red-50 rounded transition-colors'>Remove</button>"
			out += "</div>"
		}
		out += "</div>"

		c.Set("Content-Type", "text/html")
		return c.SendString(out)
	}
}

func CreateSiteEnvVar(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		key := strings.TrimSpace(c.FormValue("key"))
		value := c.FormValue("value")

		if key == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Key is required")
		}

		// Validate key format (alphanumeric + underscores only)
		for _, ch := range key {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_') {
				return c.Status(fiber.StatusBadRequest).SendString("Key must contain only letters, numbers, and underscores")
			}
		}

		if err := models.CreateEnvVar(db, id, key, value); err != nil {
			log.Printf("failed to create env var for site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save environment variable")
		}

		models.LogActivityWithContext(db, "site", id, "env_updated", "Set env var "+key, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("<div class='text-sm text-green-600'>Variable saved. Redeploy to apply changes.</div>")
		}
		return c.Redirect("/sites/" + strconv.Itoa(id))
	}
}

func BulkSiteAction(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		action := c.FormValue("action")
		siteIDs := c.Request().PostArgs().PeekMulti("site_ids")

		if len(siteIDs) == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("No sites selected")
		}

		var processed int
		for _, rawID := range siteIDs {
			id, err := strconv.Atoi(string(rawID))
			if err != nil {
				continue
			}

			site, err := models.GetSiteByID(db, id)
			if err != nil {
				continue
			}

			switch action {
			case "start":
				if site.IsLocal && site.ComposePath != "" {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					err = docker.LocalComposeStart(ctx, site.ComposePath)
					cancel()
				} else if site.ServerID.Valid {
					server, sErr := models.GetServerByID(db, int(site.ServerID.Int64))
					if sErr == nil {
						err = docker.StartSiteRemote(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName)
					}
				}
				if err == nil {
					_ = models.UpdateSiteStatus(db, id, "running")
					models.LogActivityWithContext(db, "site", id, "started", "Bulk started site "+site.Domain, c.IP(), c.Get("User-Agent"))
				}
			case "stop":
				if site.IsLocal && site.ComposePath != "" {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					err = docker.LocalComposeStop(ctx, site.ComposePath)
					cancel()
				} else if site.ServerID.Valid {
					server, sErr := models.GetServerByID(db, int(site.ServerID.Int64))
					if sErr == nil {
						err = docker.StopSiteRemote(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName)
					}
				}
				if err == nil {
					_ = models.UpdateSiteStatus(db, id, "stopped")
					models.LogActivityWithContext(db, "site", id, "stopped", "Bulk stopped site "+site.Domain, c.IP(), c.Get("User-Agent"))
				}
			case "restart":
				if site.IsLocal && site.ComposePath != "" {
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					err = docker.LocalComposeRestart(ctx, site.ComposePath)
					cancel()
				} else if site.ServerID.Valid {
					server, sErr := models.GetServerByID(db, int(site.ServerID.Int64))
					if sErr == nil {
						err = docker.RestartSiteRemote(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey, site.ContainerName)
					}
				}
				if err == nil {
					_ = models.UpdateSiteStatus(db, id, "running")
					models.LogActivityWithContext(db, "site", id, "restarted", "Bulk restarted site "+site.Domain, c.IP(), c.Get("User-Agent"))
				}
			default:
				return c.Status(fiber.StatusBadRequest).SendString("Invalid action: " + action)
			}

			if err != nil {
				log.Printf("bulk %s failed for site %d (%s): %v", action, id, site.Domain, err)
			} else {
				processed++
			}
		}

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/sites")
			return c.SendString("")
		}
		return c.Redirect("/sites")
	}
}

func DeleteSiteEnvVar(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		siteID, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		varID, err := strconv.Atoi(c.Params("varId"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid variable ID")
		}

		if err := models.DeleteEnvVar(db, varID); err != nil {
			log.Printf("failed to delete env var %d: %v", varID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete variable")
		}

		models.LogActivityWithContext(db, "site", siteID, "env_deleted", "Removed env var", c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/sites/" + strconv.Itoa(siteID))
	}
}

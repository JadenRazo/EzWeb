package handlers

import (
	"context"
	"database/sql"
	"log"
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

func ListSites(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sites, err := models.GetAllSites(db)
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
		return pages.Sites(sites, servers, templates, customers).Render(c.Context(), c.Response().BodyWriter())
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

		port, err := strconv.Atoi(c.FormValue("port", "0"))
		if err != nil || port == 0 {
			port, err = nextAvailablePort(db)
			if err != nil {
				log.Printf("failed to assign port: %v", err)
				port = 8080
			}
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

		models.LogActivity(db, "site", site.ID, "created", "Created site "+site.Domain)

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
				return c.Status(fiber.StatusInternalServerError).SendString("Deploy failed: " + err.Error())
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("No server assigned to this site")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusNotFound).SendString("Assigned server not found")
			}

			if err := docker.DeploySite(
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath,
				site.Domain, site.TemplateSlug, site.ContainerName, site.Port,
			); err != nil {
				log.Printf("deploy failed for site %d (%s): %v", id, site.Domain, err)
				_ = models.UpdateSiteStatus(db, id, "error")
				return c.Status(fiber.StatusInternalServerError).SendString("Deployment failed: " + err.Error())
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivity(db, "site", id, "deployed", "Deployed site "+site.Domain)

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
				return c.Status(fiber.StatusInternalServerError).SendString("Start failed: " + err.Error())
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
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, site.ContainerName,
			); err != nil {
				log.Printf("start failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Start failed: " + err.Error())
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivity(db, "site", id, "started", "Started site "+site.Domain)

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
				return c.Status(fiber.StatusInternalServerError).SendString("Stop failed: " + err.Error())
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
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, site.ContainerName,
			); err != nil {
				log.Printf("stop failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Stop failed: " + err.Error())
			}
		}

		_ = models.UpdateSiteStatus(db, id, "stopped")
		models.LogActivity(db, "site", id, "stopped", "Stopped site "+site.Domain)

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
				return c.Status(fiber.StatusInternalServerError).SendString("Restart failed: " + err.Error())
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
				server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, site.ContainerName,
			); err != nil {
				log.Printf("restart failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Restart failed: " + err.Error())
			}
		}

		_ = models.UpdateSiteStatus(db, id, "running")
		models.LogActivity(db, "site", id, "restarted", "Restarted site "+site.Domain)

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
					server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, site.ContainerName,
				); rmErr != nil {
					log.Printf("remote cleanup failed for site %d: %v (continuing with DB delete)", id, rmErr)
				}
			}
		}

		domain := site.Domain

		if err := models.DeleteSite(db, id); err != nil {
			log.Printf("failed to delete site %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete site")
		}
		models.LogActivity(db, "site", id, "deleted", "Deleted site "+domain)

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

		models.LogActivity(db, "site", id, "updated", "Updated site "+domain)

		// Trigger Caddy reload if domain or routing changed
		domainChanged := domain != existing.Domain
		if caddyMgr != nil && domainChanged {
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

func nextAvailablePort(db *sql.DB) (int, error) {
	var maxPort sql.NullInt64
	err := db.QueryRow("SELECT MAX(port) FROM sites").Scan(&maxPort)
	if err != nil {
		return 8080, err
	}
	if !maxPort.Valid || maxPort.Int64 < 8080 {
		return 8080, nil
	}
	return int(maxPort.Int64) + 1, nil
}

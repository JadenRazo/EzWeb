package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"html"
	"log"
	"strings"
	"time"

	"ezweb/internal/caddy"
	"ezweb/internal/docker"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func ImportPage() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return pages.ImportPage(nil).Render(c.Context(), c.Response().BodyWriter())
	}
}

func ScanProjects(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		projects, err := docker.ScanLocalProjects(ctx)
		if err != nil {
			log.Printf("scan failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to scan: " + err.Error())
		}

		// Filter out projects already managed by EzWeb
		var available []docker.ScannedProject
		for _, p := range projects {
			if p.Path == "" {
				continue
			}
			_, err := models.GetSiteByComposePath(db, p.Path)
			if err != nil {
				// Not found = available for import
				available = append(available, p)
			}
		}

		c.Set("Content-Type", "text/html")
		return pages.ImportScanResults(available).Render(c.Context(), c.Response().BodyWriter())
	}
}

func ImportProject(db *sql.DB, caddyMgr *caddy.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		composePath := strings.TrimSpace(c.FormValue("compose_path"))
		domain := strings.TrimSpace(c.FormValue("domain"))
		routingJSON := strings.TrimSpace(c.FormValue("routing_config"))

		if composePath == "" || domain == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Compose path and domain are required")
		}

		containerName := strings.ReplaceAll(domain, ".", "-")

		var routingConfig *models.RoutingConfig
		if routingJSON != "" {
			var rc models.RoutingConfig
			if err := json.Unmarshal([]byte(routingJSON), &rc); err != nil {
				return c.Status(fiber.StatusBadRequest).SendString("Invalid routing config JSON: " + err.Error())
			}
			routingConfig = &rc
		}

		site := &models.Site{
			Domain:        domain,
			ContainerName: containerName,
			Status:        "running",
			IsLocal:       true,
			ComposePath:   composePath,
			RoutingConfig: routingConfig,
		}

		if err := models.CreateSite(db, site); err != nil {
			log.Printf("failed to import project: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to import: " + err.Error())
		}

		// Trigger Caddy reload
		if caddyMgr != nil {
			if err := caddyMgr.AddSite(db, *site); err != nil {
				log.Printf("caddy reload failed after importing %s: %v", domain, err)
			}
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return c.SendString(`<div class="p-4 bg-green-50 text-green-800 rounded-lg">Imported ` + html.EscapeString(domain) + ` successfully. <a href="/sites" class="underline font-medium">View sites</a></div>`)
		}
		return c.Redirect("/sites")
	}
}

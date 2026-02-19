package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
)

// PublicStatus returns a JSON array of all site statuses with their latest
// health check data. This endpoint is unauthenticated and intended for
// external consumption (e.g. profile README deploy monitors).
func PublicStatus(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("failed to list sites for public status: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to load site status",
			})
		}

		type statusJSON struct {
			Domain          string `json:"domain"`
			Status          string `json:"status"`
			Template        string `json:"template"`
			HTTPStatus      int    `json:"http_status"`
			LatencyMs       int    `json:"latency_ms"`
			ContainerStatus string `json:"container_status"`
			CheckedAt       string `json:"checked_at"`
		}

		result := make([]statusJSON, 0, len(sites))
		hidden := 0
		for _, site := range sites {
			domain := site.Domain
			if !strings.Contains(domain, "jadenrazo.dev") {
				hidden++
				domain = fmt.Sprintf("client-site-%d.example", hidden)
			}

			entry := statusJSON{
				Domain:   domain,
				Status:   site.Status,
				Template: site.TemplateSlug,
			}

			hc, err := models.GetLatestHealthCheck(db, site.ID)
			if err == nil {
				entry.HTTPStatus = hc.HTTPStatus
				entry.LatencyMs = hc.LatencyMs
				entry.ContainerStatus = hc.ContainerStatus
				entry.CheckedAt = hc.CheckedAt
			}

			result = append(result, entry)
		}

		c.Set("Cache-Control", "public, max-age=300")
		c.Set("Last-Modified", time.Now().UTC().Format(time.RFC1123))
		return c.JSON(result)
	}
}

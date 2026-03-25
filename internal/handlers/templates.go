package handlers

import (
	"database/sql"
	"log"

	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
)

// ListTemplates returns a JSON list of available site templates.
func ListTemplates(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		templates, err := models.GetAllTemplates(db)
		if err != nil {
			log.Printf("failed to list templates: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to load templates",
			})
		}

		type templateJSON struct {
			ID          int    `json:"id"`
			Slug        string `json:"slug"`
			Label       string `json:"label"`
			Description string `json:"description"`
		}

		result := make([]templateJSON, len(templates))
		for i, t := range templates {
			result[i] = templateJSON{
				ID:          t.ID,
				Slug:        t.Slug,
				Label:       t.Label,
				Description: t.Description,
			}
		}

		return c.JSON(result)
	}
}

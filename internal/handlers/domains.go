package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"strings"

	"ezweb/internal/domain"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

// DomainsPage handles GET /domains.  It renders the domain search page with an
// empty results set ready for the user's first query.
func DomainsPage(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return pages.Domains(nil, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

// SearchDomains handles POST /domains/search.
// It accepts a form field "domain" (the search query), fans out to all
// configured providers via the Manager, and returns an HTMX partial with the
// results table.
func SearchDomains(mgr *domain.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		query := strings.TrimSpace(c.FormValue("domain"))
		if query == "" {
			c.Set("Content-Type", "text/html")
			return partials.DomainResults(nil, "").Render(c.Context(), c.Response().BodyWriter())
		}

		// Strip protocol/www prefix if the user typed a full URL.
		query = strings.TrimPrefix(query, "https://")
		query = strings.TrimPrefix(query, "http://")
		query = strings.TrimPrefix(query, "www.")
		query = strings.ToLower(query)
		// Remove any path component.
		if idx := strings.Index(query, "/"); idx != -1 {
			query = query[:idx]
		}

		results, err := mgr.Search(c.Context(), query)
		if err != nil {
			log.Printf("domain search failed for %q: %v", query, err)
			c.Set("Content-Type", "text/html")
			return partials.DomainResults(nil, query).Render(c.Context(), c.Response().BodyWriter())
		}

		c.Set("Content-Type", "text/html")
		return partials.DomainResults(results, query).Render(c.Context(), c.Response().BodyWriter())
	}
}

// SearchDomainsJSON handles POST /api/domains/search.
// It returns JSON results for use by the quote form's domain picker widget.
func SearchDomainsJSON(mgr *domain.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		query := strings.TrimSpace(c.FormValue("domain"))
		if query == "" {
			return c.JSON([]domain.DomainResult{})
		}

		query = strings.TrimPrefix(query, "https://")
		query = strings.TrimPrefix(query, "http://")
		query = strings.TrimPrefix(query, "www.")
		query = strings.ToLower(query)
		if idx := strings.Index(query, "/"); idx != -1 {
			query = query[:idx]
		}

		results, err := mgr.Search(c.Context(), query)
		if err != nil {
			log.Printf("domain JSON search failed for %q: %v", query, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "search failed",
			})
		}

		// Use json.Marshal so we keep the standard library; fiber's c.JSON
		// also uses encoding/json internally, but we set Content-Type
		// explicitly for clarity.
		out, err := json.Marshal(results)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "encode failed",
			})
		}

		c.Set("Content-Type", "application/json")
		return c.Send(out)
	}
}

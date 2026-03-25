package handlers

import (
	"database/sql"
	"fmt"
	"html"
	"log"
	"strconv"
	"strings"
	"time"

	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

// CreateQuoteRequestAPI handles POST /quote-requests (JSON).
// Public API endpoint consumed by external marketing sites.
func CreateQuoteRequestAPI(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		var body struct {
			Name        string `json:"name"`
			Email       string `json:"email"`
			Phone       string `json:"phone"`
			Company     string `json:"company"`
			ProjectType string `json:"projectType"`
			Description string `json:"description"`
		}

		if err := c.BodyParser(&body); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"success": false,
				"error":   "Invalid JSON body",
			})
		}

		body.Name = strings.TrimSpace(body.Name)
		body.Email = strings.TrimSpace(body.Email)
		body.Phone = strings.TrimSpace(body.Phone)
		body.Company = strings.TrimSpace(body.Company)
		body.ProjectType = strings.TrimSpace(body.ProjectType)
		body.Description = strings.TrimSpace(body.Description)

		if body.Name == "" || body.Email == "" {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"success": false,
				"error":   "Name and email are required",
			})
		}

		if len(body.Name) > 200 || len(body.Email) > 254 || len(body.Phone) > 30 ||
			len(body.Company) > 200 || len(body.ProjectType) > 100 || len(body.Description) > 2000 {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"success": false,
				"error":   "One or more fields exceed maximum length",
			})
		}

		if !validateEmail(body.Email) {
			return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
				"success": false,
				"error":   "Please enter a valid email address",
			})
		}

		req := &models.QuoteRequest{
			Name:        body.Name,
			Email:       body.Email,
			Phone:       body.Phone,
			Company:     body.Company,
			ProjectType: body.ProjectType,
			Description: body.Description,
		}

		if err := models.CreateQuoteRequest(db, req); err != nil {
			log.Printf("quote request API: failed to store request from %s: %v", sanitizeLogInput(body.Email), err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"success": false,
				"error":   "Failed to save quote request",
			})
		}

		models.LogActivityWithContext(db, "quote_request", req.ID, "created",
			"New quote request from "+sanitizeLogInput(body.Name)+" ("+sanitizeLogInput(body.Email)+") via API",
			c.IP(), c.Get("User-Agent"))

		return c.Status(fiber.StatusCreated).JSON(fiber.Map{
			"success": true,
			"message": "Quote request received",
		})
	}
}

// ListQuoteRequests handles GET /quote-requests.
// Supports optional ?status= filtering and ?page= pagination.
func ListQuoteRequests(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}

		statusFilter := strings.TrimSpace(c.Query("status", ""))
		offset := (page - 1) * perPage

		var (
			requests []models.QuoteRequest
			total    int
			err      error
		)

		if statusFilter != "" {
			total, err = models.CountQuoteRequestsByStatus(db, statusFilter)
			if err != nil {
				log.Printf("failed to count quote requests by status %q: %v", statusFilter, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quote requests")
			}
			requests, err = models.GetQuoteRequestsByStatusPaginated(db, statusFilter, perPage, offset)
			if err != nil {
				log.Printf("failed to list quote requests by status %q: %v", statusFilter, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quote requests")
			}
		} else {
			total, err = models.CountQuoteRequests(db)
			if err != nil {
				log.Printf("failed to count quote requests: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quote requests")
			}
			requests, err = models.GetQuoteRequestsPaginated(db, perPage, offset)
			if err != nil {
				log.Printf("failed to list quote requests: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quote requests")
			}
		}

		// Build per-status counts for the filter tabs.
		counts := map[string]int{
			"all": 0,
		}
		allCount, _ := models.CountQuoteRequests(db)
		counts["all"] = allCount
		for _, s := range []string{"new", "contacted", "converted", "archived"} {
			n, _ := models.CountQuoteRequestsByStatus(db, s)
			counts[s] = n
		}

		c.Set("Content-Type", "text/html")
		return pages.QuoteRequests(requests, statusFilter, page, total, perPage, counts).Render(c.Context(), c.Response().BodyWriter())
	}
}

// UpdateQuoteRequestStatus handles PUT /quote-requests/:id/status.
// Accepts the new status value from form body or JSON.
func UpdateQuoteRequestStatus(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote request ID")
		}

		req, err := models.GetQuoteRequestByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote request not found")
		}

		status := strings.TrimSpace(c.FormValue("status"))
		if status == "" {
			// Try JSON body as well.
			var body struct {
				Status string `json:"status"`
			}
			if err := c.BodyParser(&body); err == nil {
				status = body.Status
			}
		}

		validStatuses := map[string]bool{"new": true, "contacted": true, "converted": true, "archived": true}
		if !validStatuses[status] {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid status value")
		}

		if err := models.UpdateQuoteRequestStatus(db, id, status); err != nil {
			log.Printf("failed to update quote request %d status: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update status")
		}

		models.LogActivityWithContext(db, "quote_request", id, "status_updated",
			fmt.Sprintf("Quote request from %s marked as %s", sanitizeLogInput(req.Name), sanitizeLogInput(status)),
			c.IP(), c.Get("User-Agent"))

		// Reload and re-render the updated row.
		updated, err := models.GetQuoteRequestByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Updated but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return renderQuoteRequestRow(c, updated)
		}
		return c.Redirect("/quote-requests")
	}
}

// ConvertQuoteRequest handles POST /quote-requests/:id/convert.
// Creates a draft Quote pre-populated from the quote request's fields, then
// marks the request as "converted".
func ConvertQuoteRequest(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote request ID")
		}

		req, err := models.GetQuoteRequestByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote request not found")
		}

		validityDays := models.GetQuoteValidityDays(db)
		taxRate := models.GetTaxRate(db)

		q := &models.Quote{
			CustomerName:    req.Name,
			CustomerEmail:   req.Email,
			CustomerPhone:   req.Phone,
			CustomerCompany: req.Company,
			Notes:           req.Description,
			BillingCycle:    "monthly",
			Status:          "draft",
			TaxRate:         taxRate,
			ValidUntil:      time.Now().AddDate(0, 0, validityDays).Format("2006-01-02"),
		}

		if err := models.CreateQuote(db, q); err != nil {
			log.Printf("failed to create quote from request %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create quote")
		}

		if err := models.UpdateQuoteRequestStatus(db, id, "converted"); err != nil {
			log.Printf("failed to mark quote request %d as converted: %v", id, err)
		}

		models.LogActivityWithContext(db, "quote_request", id, "converted",
			fmt.Sprintf("Quote request from %s converted to quote %d", sanitizeLogInput(req.Name), q.ID),
			c.IP(), c.Get("User-Agent"))

		// If HTMX, return the updated row; otherwise redirect to the new quote.
		if c.Get("HX-Request") != "" {
			updated, err := models.GetQuoteRequestByID(db, id)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Converted but failed to reload")
			}
			c.Set("Content-Type", "text/html")
			return renderQuoteRequestRow(c, updated)
		}

		return c.Redirect(fmt.Sprintf("/quotes/%d", q.ID))
	}
}

// DeleteQuoteRequest handles DELETE /quote-requests/:id.
func DeleteQuoteRequest(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote request ID")
		}

		req, err := models.GetQuoteRequestByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote request not found")
		}

		if err := models.DeleteQuoteRequest(db, id); err != nil {
			log.Printf("failed to delete quote request %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete quote request")
		}

		models.LogActivityWithContext(db, "quote_request", id, "deleted",
			fmt.Sprintf("Deleted quote request from %s", sanitizeLogInput(req.Name)),
			c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			// Also remove the optional description row that follows in the DOM.
			return c.SendString("")
		}
		return c.Redirect("/quote-requests")
	}
}

// renderQuoteRequestRow writes a single table row for a quote request as HTML.
// Used by HTMX swap responses after status changes.
func renderQuoteRequestRow(c *fiber.Ctx, req *models.QuoteRequest) error {
	// Inline the row HTML rather than creating a dedicated partial to keep the
	// template count manageable. The row mirrors the structure in quote_requests.templ.
	statusColor := map[string]string{
		"new":       "blue",
		"contacted": "yellow",
		"converted": "green",
		"archived":  "gray",
	}
	color := statusColor[req.Status]
	if color == "" {
		color = "gray"
	}

	html := fmt.Sprintf(`<tr id="qreq-%d" class="border-b border-gray-100 hover:bg-gray-50/80 transition-colors duration-100">
	<td class="px-6 py-4 text-sm font-medium text-gray-900">%s</td>
	<td class="px-6 py-4 text-sm text-gray-600"><a href="mailto:%s" class="hover:text-blue-600 transition-colors">%s</a></td>
	<td class="px-6 py-4 text-sm text-gray-600">%s</td>
	<td class="px-6 py-4 text-sm text-gray-600">%s</td>
	<td class="px-6 py-4"><span class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-full text-xs font-medium whitespace-nowrap %s"><span class="w-1.5 h-1.5 rounded-full flex-shrink-0 %s"></span>%s</span></td>
	<td class="px-6 py-4 text-sm text-gray-500">%s</td>
	<td class="px-6 py-4 text-right text-sm text-gray-400 italic">Updated</td>
</tr>`,
		req.ID,
		html.EscapeString(req.Name),
		html.EscapeString(req.Email), html.EscapeString(req.Email),
		html.EscapeString(req.ProjectType),
		html.EscapeString(req.BudgetRange),
		badgeBgClass(color), badgeDotClass(color),
		html.EscapeString(req.Status),
		req.CreatedAt.Format("Jan 2, 2006"),
	)

	return c.SendString(html)
}

func badgeBgClass(color string) string {
	switch color {
	case "blue":
		return "bg-blue-50 text-blue-700 ring-1 ring-blue-200/60"
	case "yellow":
		return "bg-yellow-50 text-yellow-700 ring-1 ring-yellow-200/60"
	case "green":
		return "bg-green-50 text-green-700 ring-1 ring-green-200/60"
	case "red":
		return "bg-red-50 text-red-700 ring-1 ring-red-200/60"
	default:
		return "bg-gray-100 text-gray-600 ring-1 ring-gray-200/60"
	}
}

func badgeDotClass(color string) string {
	switch color {
	case "blue":
		return "bg-blue-500"
	case "yellow":
		return "bg-yellow-500"
	case "green":
		return "bg-green-500"
	case "red":
		return "bg-red-500"
	default:
		return "bg-gray-400"
	}
}


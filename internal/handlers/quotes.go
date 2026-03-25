package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezweb/internal/models"
	quotepkg "ezweb/internal/quote"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

// ListQuotes handles GET /quotes.
// Supports optional ?status= filtering and ?page= pagination.
func ListQuotes(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}

		statusFilter := strings.TrimSpace(c.Query("status", ""))
		offset := (page - 1) * perPage

		var (
			quotes []models.Quote
			total  int
			err    error
		)

		if statusFilter != "" {
			// Filtered path: count by status and query with WHERE clause.
			total, err = models.CountQuotesByStatus(db, statusFilter)
			if err != nil {
				log.Printf("failed to count quotes by status %q: %v", statusFilter, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quotes")
			}
			quotes, err = getQuotesByStatusPaginated(db, statusFilter, perPage, offset)
			if err != nil {
				log.Printf("failed to list quotes by status %q: %v", statusFilter, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quotes")
			}
		} else {
			total, err = models.CountQuotes(db)
			if err != nil {
				log.Printf("failed to count quotes: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quotes")
			}
			quotes, err = models.GetQuotesPaginated(db, perPage, offset)
			if err != nil {
				log.Printf("failed to list quotes: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to load quotes")
			}
		}

		tiers, err := models.GetAllPricingTiers(db)
		if err != nil {
			log.Printf("failed to load pricing tiers for quote form: %v", err)
		}

		addons, err := models.GetAllAddons(db)
		if err != nil {
			log.Printf("failed to load addons for quote form: %v", err)
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers for quote form: %v", err)
		}

		c.Set("Content-Type", "text/html")
		return pages.Quotes(quotes, statusFilter, page, total, perPage, tiers, addons, customers).Render(c.Context(), c.Response().BodyWriter())
	}
}

// QuoteDetail handles GET /quotes/:id.
func QuoteDetail(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		q, err := models.GetQuoteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		tiers, err := models.GetAllPricingTiers(db)
		if err != nil {
			log.Printf("failed to load pricing tiers for quote detail: %v", err)
		}

		addons, err := models.GetAllAddons(db)
		if err != nil {
			log.Printf("failed to load addons for quote detail: %v", err)
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers for quote detail: %v", err)
		}

		c.Set("Content-Type", "text/html")
		return pages.QuoteDetail(q, tiers, addons, customers).Render(c.Context(), c.Response().BodyWriter())
	}
}

// CreateQuote handles POST /quotes.
func CreateQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		q := &models.Quote{
			CustomerName:    c.FormValue("customer_name"),
			CustomerEmail:   c.FormValue("customer_email"),
			CustomerPhone:   c.FormValue("customer_phone"),
			CustomerCompany: c.FormValue("customer_company"),
			TemplateSlug:    c.FormValue("template_slug"),
			DomainName:      c.FormValue("domain_name"),
			DomainRegistrar: c.FormValue("domain_registrar"),
			BillingCycle:    c.FormValue("billing_cycle"),
			Notes:           c.FormValue("notes"),
			Status:          "draft",
		}

		if q.CustomerName == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Customer name is required")
		}

		if !validateNotes(q.Notes) {
			return c.Status(fiber.StatusBadRequest).SendString("Notes must be 1000 characters or less")
		}

		// If a known customer is selected, pull their contact info.
		if cidStr := c.FormValue("customer_id"); cidStr != "" {
			cid, err := strconv.Atoi(cidStr)
			if err == nil && cid > 0 {
				customer, err := models.GetCustomerByID(db, cid)
				if err == nil {
					q.CustomerID = sql.NullInt64{Int64: int64(cid), Valid: true}
					// Only override blanks so manual overrides are respected.
					if q.CustomerName == "" {
						q.CustomerName = customer.Name
					}
					if q.CustomerEmail == "" {
						q.CustomerEmail = customer.Email
					}
					if q.CustomerPhone == "" {
						q.CustomerPhone = customer.Phone
					}
					if q.CustomerCompany == "" {
						q.CustomerCompany = customer.Company
					}
				}
			}
		}

		// Domain price — optional float.
		if v := c.FormValue("domain_price"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				q.DomainPrice = f
			}
		}

		// Discount — optional float, capped at [0, 100].
		if v := c.FormValue("discount_percent"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
				q.DiscountPercent = f
			}
		}

		// Billing cycle default.
		if q.BillingCycle == "" {
			q.BillingCycle = "monthly"
		}

		// Pricing tier — look up by template slug to fill setup/recurring prices.
		if q.TemplateSlug != "" {
			tier, err := models.GetPricingTierBySlug(db, q.TemplateSlug)
			if err == nil {
				q.SetupFee = tier.SetupFee
				q.MonthlyPrice = tier.MonthlyPrice
				q.YearlyPrice = tier.YearlyPrice
			} else {
				log.Printf("pricing tier not found for slug %q: %v", q.TemplateSlug, err)
			}
		}

		// Tax rate and validity from settings.
		q.TaxRate = models.GetTaxRate(db)
		validityDays := models.GetQuoteValidityDays(db)
		q.ValidUntil = time.Now().AddDate(0, 0, validityDays).Format("2006-01-02")

		// Parse addons — repeating field addon_ids[] with per-addon quantity fields.
		q.Addons = parseAddonLines(c, db)

		models.CalculateQuoteTotals(q)

		if err := models.CreateQuote(db, q); err != nil {
			log.Printf("failed to create quote: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create quote")
		}

		// Persist addon lines now that we have the quote ID.
		for i := range q.Addons {
			q.Addons[i].QuoteID = q.ID
			if err := models.AddQuoteAddon(db, &q.Addons[i]); err != nil {
				log.Printf("failed to add addon to quote %d: %v", q.ID, err)
			}
		}

		models.LogActivityWithContext(db, "quote", q.ID, "created",
			fmt.Sprintf("Created quote for %s", sanitizeLogInput(q.CustomerName)),
			c.IP(), c.Get("User-Agent"))

		// Reload to get joined fields and generated timestamps.
		created, err := models.GetQuoteByID(db, q.ID)
		if err != nil {
			log.Printf("failed to reload quote %d after create: %v", q.ID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Quote created but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.QuoteRow(*created).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/quotes")
	}
}

// UpdateQuote handles PUT /quotes/:id.
func UpdateQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		existing, err := models.GetQuoteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		q := &models.Quote{
			ID:              id,
			PublicID:        existing.PublicID,
			Status:          existing.Status,
			CustomerName:    c.FormValue("customer_name"),
			CustomerEmail:   c.FormValue("customer_email"),
			CustomerPhone:   c.FormValue("customer_phone"),
			CustomerCompany: c.FormValue("customer_company"),
			TemplateSlug:    c.FormValue("template_slug"),
			DomainName:      c.FormValue("domain_name"),
			DomainRegistrar: c.FormValue("domain_registrar"),
			BillingCycle:    c.FormValue("billing_cycle"),
			Notes:           c.FormValue("notes"),
			ValidUntil:      existing.ValidUntil,
		}

		if q.CustomerName == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Customer name is required")
		}

		if !validateNotes(q.Notes) {
			return c.Status(fiber.StatusBadRequest).SendString("Notes must be 1000 characters or less")
		}

		// Linked customer.
		if cidStr := c.FormValue("customer_id"); cidStr != "" {
			cid, err := strconv.Atoi(cidStr)
			if err == nil && cid > 0 {
				q.CustomerID = sql.NullInt64{Int64: int64(cid), Valid: true}
			}
		} else {
			q.CustomerID = existing.CustomerID
		}

		// Domain price.
		if v := c.FormValue("domain_price"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
				q.DomainPrice = f
			}
		} else {
			q.DomainPrice = existing.DomainPrice
		}

		// Discount.
		if v := c.FormValue("discount_percent"); v != "" {
			if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 && f <= 100 {
				q.DiscountPercent = f
			}
		} else {
			q.DiscountPercent = existing.DiscountPercent
		}

		if q.BillingCycle == "" {
			q.BillingCycle = existing.BillingCycle
		}

		// Pricing tier.
		if q.TemplateSlug != "" {
			tier, err := models.GetPricingTierBySlug(db, q.TemplateSlug)
			if err == nil {
				q.SetupFee = tier.SetupFee
				q.MonthlyPrice = tier.MonthlyPrice
				q.YearlyPrice = tier.YearlyPrice
			} else {
				log.Printf("pricing tier not found for slug %q: %v", q.TemplateSlug, err)
				q.SetupFee = existing.SetupFee
				q.MonthlyPrice = existing.MonthlyPrice
				q.YearlyPrice = existing.YearlyPrice
			}
		} else {
			q.SetupFee = existing.SetupFee
			q.MonthlyPrice = existing.MonthlyPrice
			q.YearlyPrice = existing.YearlyPrice
		}

		q.TaxRate = models.GetTaxRate(db)

		// Re-parse addons, replacing old set.
		q.Addons = parseAddonLines(c, db)

		models.CalculateQuoteTotals(q)

		if err := models.UpdateQuote(db, q); err != nil {
			log.Printf("failed to update quote %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update quote")
		}

		// Replace addon lines atomically.
		if err := models.RemoveQuoteAddons(db, id); err != nil {
			log.Printf("failed to remove old addons for quote %d: %v", id, err)
		}
		for i := range q.Addons {
			q.Addons[i].QuoteID = id
			if err := models.AddQuoteAddon(db, &q.Addons[i]); err != nil {
				log.Printf("failed to add addon to quote %d: %v", id, err)
			}
		}

		models.LogActivityWithContext(db, "quote", id, "updated",
			fmt.Sprintf("Updated quote for %s", sanitizeLogInput(q.CustomerName)),
			c.IP(), c.Get("User-Agent"))

		updated, err := models.GetQuoteByID(db, id)
		if err != nil {
			log.Printf("failed to reload quote %d after update: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Quote updated but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.QuoteRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/quotes/" + strconv.Itoa(id))
	}
}

// DeleteQuote handles DELETE /quotes/:id.
func DeleteQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		q, err := models.GetQuoteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		if err := models.DeleteQuote(db, id); err != nil {
			log.Printf("failed to delete quote %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete quote")
		}

		models.LogActivityWithContext(db, "quote", id, "deleted",
			fmt.Sprintf("Deleted quote for %s", sanitizeLogInput(q.CustomerName)),
			c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/quotes")
	}
}

// SendQuote handles POST /quotes/:id/send.
// Transitions the quote to "sent" status and returns the updated row.
func SendQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		if _, err := models.GetQuoteByID(db, id); err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		if err := models.UpdateQuoteStatus(db, id, "sent"); err != nil {
			log.Printf("failed to mark quote %d as sent: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to send quote")
		}

		models.LogActivityWithContext(db, "quote", id, "sent",
			"Quote marked as sent", c.IP(), c.Get("User-Agent"))

		updated, err := models.GetQuoteByID(db, id)
		if err != nil {
			log.Printf("failed to reload quote %d after send: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Quote sent but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.QuoteRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/quotes/" + strconv.Itoa(id))
	}
}

// QuotePDF handles GET /quotes/:id/pdf.
// Streams the generated PDF inline for browser viewing.
func QuotePDF(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		q, err := models.GetQuoteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		pdfBytes, err := quotepkg.GeneratePDF(db, q)
		if err != nil {
			log.Printf("failed to generate PDF for quote %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate PDF")
		}

		filename := fmt.Sprintf("quote-%s.pdf", q.PublicID[:8])
		c.Set("Content-Type", "application/pdf")
		c.Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", filename))
		return c.Send(pdfBytes)
	}
}

// PublicQuote handles GET /q/:publicId (no auth required).
// Renders a clean customer-facing view of the quote.
func PublicQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		publicID := c.Params("publicId")
		if publicID == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote link")
		}

		q, err := models.GetQuoteByPublicID(db, publicID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found or link has expired")
		}

		settings, err := models.GetAllSettings(db)
		if err != nil {
			log.Printf("failed to load settings for public quote %s: %v", publicID, err)
			settings = make(map[string]string)
		}

		c.Set("Content-Type", "text/html")
		return pages.QuotePublic(q, settings).Render(c.Context(), c.Response().BodyWriter())
	}
}

// AcceptQuote handles POST /q/:publicId/accept (no auth required).
// Only quotes with status "sent" may be accepted.
func AcceptQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		publicID := c.Params("publicId")
		if publicID == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote link")
		}

		q, err := models.GetQuoteByPublicID(db, publicID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		if q.Status != "sent" {
			return c.Status(fiber.StatusBadRequest).SendString("This quote cannot be accepted in its current state")
		}

		if err := models.UpdateQuoteStatus(db, q.ID, "accepted"); err != nil {
			log.Printf("failed to accept quote %d: %v", q.ID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to accept quote")
		}

		models.LogActivityWithContext(db, "quote", q.ID, "accepted",
			fmt.Sprintf("Quote accepted by %s", sanitizeLogInput(q.CustomerName)),
			c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return c.SendString(`<div class="text-green-600 font-semibold">Thank you! Your quote has been accepted. We will be in touch shortly.</div>`)
		}
		return c.Redirect("/q/"+publicID)
	}
}

// ConvertQuote handles POST /quotes/:id/convert.
// Requires the quote to be in "accepted" status. Creates a customer (if one is
// not already linked), a site record, and a payment for the setup fee, then
// marks the quote as converted and links it to the new site.
func ConvertQuote(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid quote ID")
		}

		q, err := models.GetQuoteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Quote not found")
		}

		if q.Status != "accepted" {
			return c.Status(fiber.StatusBadRequest).SendString("Only accepted quotes can be converted")
		}

		// Determine or create the customer record.
		var customerID int
		if q.CustomerID.Valid && q.CustomerID.Int64 > 0 {
			customerID = int(q.CustomerID.Int64)
		} else {
			newCustomer := &models.Customer{
				Name:    q.CustomerName,
				Email:   q.CustomerEmail,
				Phone:   q.CustomerPhone,
				Company: q.CustomerCompany,
			}
			if err := models.CreateCustomer(db, newCustomer); err != nil {
				log.Printf("failed to create customer during quote %d conversion: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to create customer record")
			}
			customerID = newCustomer.ID
			models.LogActivityWithContext(db, "customer", customerID, "created",
				fmt.Sprintf("Customer created from quote conversion: %s", sanitizeLogInput(q.CustomerName)),
				c.IP(), c.Get("User-Agent"))
		}

		// Build and persist the site record.
		domain := strings.TrimSpace(q.DomainName)
		if domain == "" {
			domain = strings.ToLower(strings.ReplaceAll(q.CustomerName, " ", "-"))
		}

		containerName := strings.ReplaceAll(domain, ".", "-")

		port, err := nextAvailablePort(db)
		if err != nil {
			log.Printf("failed to assign port for converted site (quote %d): %v", id, err)
			port = 8080
		}

		site := &models.Site{
			Domain:        domain,
			TemplateSlug:  q.TemplateSlug,
			CustomerID:    sql.NullInt64{Int64: int64(customerID), Valid: true},
			ContainerName: containerName,
			Port:          port,
			Status:        "pending",
		}

		if err := models.CreateSite(db, site); err != nil {
			log.Printf("failed to create site during quote %d conversion: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create site record")
		}

		models.LogActivityWithContext(db, "site", site.ID, "created",
			fmt.Sprintf("Site created from quote conversion: %s", sanitizeLogInput(domain)),
			c.IP(), c.Get("User-Agent"))

		// Create a payment record for the setup fee if it is non-zero.
		if q.SetupFee > 0 {
			payment := &models.Payment{
				CustomerID: customerID,
				SiteID:     sql.NullInt64{Int64: int64(site.ID), Valid: true},
				Amount:     q.SetupFee,
				DueDate:    time.Now().Format("2006-01-02"),
				Notes:      fmt.Sprintf("Setup fee from quote %s", q.PublicID[:8]),
			}
			if err := models.CreatePayment(db, payment); err != nil {
				// Non-fatal: log and continue — the site is already created.
				log.Printf("failed to create setup payment for quote %d: %v", id, err)
			} else {
				models.LogActivityWithContext(db, "payment", payment.ID, "created",
					fmt.Sprintf("Setup fee payment created from quote conversion: $%.2f", q.SetupFee),
					c.IP(), c.Get("User-Agent"))
			}
		}

		// Mark the quote as converted and link the new site.
		if _, err := db.Exec(
			"UPDATE quotes SET status = 'converted', converted_site_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			site.ID, id,
		); err != nil {
			log.Printf("failed to update quote %d conversion status: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to finalize quote conversion")
		}

		models.LogActivityWithContext(db, "quote", id, "converted",
			fmt.Sprintf("Quote converted to site %s (ID %d)", sanitizeLogInput(domain), site.ID),
			c.IP(), c.Get("User-Agent"))

		return c.Redirect("/sites/" + strconv.Itoa(site.ID))
	}
}

// --- helpers ---

// getQuotesByStatusPaginated executes a status-filtered paginated query directly
// against the quotes table. This is a thin wrapper kept in the handler layer
// since the models layer only exposes an unfiltered variant.
func getQuotesByStatusPaginated(db *sql.DB, status string, limit, offset int) ([]models.Quote, error) {
	rows, err := db.Query(
		`SELECT
			q.id, q.public_id, q.customer_id,
			COALESCE(q.customer_name,''), COALESCE(q.customer_email,''),
			COALESCE(q.customer_phone,''), COALESCE(q.customer_company,''),
			COALESCE(q.template_slug,''), COALESCE(q.domain_name,''), COALESCE(q.domain_price,0),
			COALESCE(q.domain_registrar,''), q.setup_fee, q.monthly_price, q.yearly_price,
			COALESCE(q.billing_cycle,'monthly'), COALESCE(q.discount_percent,0), COALESCE(q.tax_rate,0),
			q.subtotal, q.tax_amount, q.total, q.status,
			COALESCE(q.notes,''), COALESCE(q.valid_until,''),
			q.sent_at, q.accepted_at, q.rejected_at, q.converted_site_id,
			q.created_at, q.updated_at,
			COALESCE(pt.label,'')
		 FROM quotes q
		 LEFT JOIN pricing_tiers pt ON q.template_slug = pt.template_slug
		 WHERE q.status = ?
		 ORDER BY q.created_at DESC
		 LIMIT ? OFFSET ?`,
		status, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query quotes by status %q: %w", status, err)
	}
	defer rows.Close()

	var quotes []models.Quote
	for rows.Next() {
		var q models.Quote
		err := rows.Scan(
			&q.ID, &q.PublicID, &q.CustomerID,
			&q.CustomerName, &q.CustomerEmail, &q.CustomerPhone, &q.CustomerCompany,
			&q.TemplateSlug, &q.DomainName, &q.DomainPrice,
			&q.DomainRegistrar, &q.SetupFee, &q.MonthlyPrice, &q.YearlyPrice,
			&q.BillingCycle, &q.DiscountPercent, &q.TaxRate,
			&q.Subtotal, &q.TaxAmount, &q.Total, &q.Status,
			&q.Notes, &q.ValidUntil,
			&q.SentAt, &q.AcceptedAt, &q.RejectedAt, &q.ConvertedSiteID,
			&q.CreatedAt, &q.UpdatedAt,
			&q.TemplateName,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quote row: %w", err)
		}
		quotes = append(quotes, q)
	}
	return quotes, rows.Err()
}

// parseAddonLines extracts addon_ids[] from the form and looks up each addon's
// price and price_type from the database. The per-addon quantity is read from
// addon_qty_{id}; it defaults to 1 if absent or unparseable.
func parseAddonLines(c *fiber.Ctx, db *sql.DB) []models.QuoteAddon {
	rawIDs := c.Request().PostArgs().PeekMulti("addon_ids[]")
	if len(rawIDs) == 0 {
		return nil
	}

	var lines []models.QuoteAddon
	for _, rawID := range rawIDs {
		addonID, err := strconv.Atoi(string(rawID))
		if err != nil || addonID <= 0 {
			continue
		}

		addon, err := models.GetAddonByID(db, addonID)
		if err != nil {
			log.Printf("addon %d not found when parsing quote form: %v", addonID, err)
			continue
		}

		qty := 1
		if v := c.FormValue(fmt.Sprintf("addon_qty_%d", addonID)); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				qty = n
			}
		}

		lines = append(lines, models.QuoteAddon{
			AddonID:   addonID,
			Quantity:  qty,
			Price:     addon.Price,
			PriceType: addon.PriceType,
			AddonName: addon.Name,
		})
	}
	return lines
}

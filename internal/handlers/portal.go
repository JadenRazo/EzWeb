package handlers

import (
	"database/sql"
	"log"
	"strings"
	"time"

	"ezweb/internal/models"
	"ezweb/internal/portal"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

// portalSettings loads business settings from the DB and provides a safe fallback.
// businessName defaults to "Our Business" when not configured so the portal
// always renders with a legible brand name.
func portalSettings(db *sql.DB) (string, map[string]string) {
	settings, err := models.GetAllSettings(db)
	if err != nil {
		log.Printf("portal: failed to load settings: %v", err)
		settings = make(map[string]string)
	}
	name := settings["business_name"]
	if name == "" {
		name = "Our Business"
	}
	return name, settings
}

// PortalHome handles GET /portal — the public landing page.
func PortalHome(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)
		tagline := settings["tagline"]
		c.Set("Content-Type", "text/html")
		return pages.PortalHome(name, tagline, settings).Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalPricing handles GET /portal/pricing — pricing tier cards.
func PortalPricing(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		tiers, err := models.GetActivePricingTiers(db)
		if err != nil {
			log.Printf("portal pricing: failed to load tiers: %v", err)
			tiers = nil
		}

		c.Set("Content-Type", "text/html")
		return pages.PortalPricing(tiers, name, settings).Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalPortfolio handles GET /portal/portfolio — visible portfolio items grid.
func PortalPortfolio(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		items, err := models.GetVisiblePortfolioItems(db)
		if err != nil {
			log.Printf("portal portfolio: failed to load items: %v", err)
			items = nil
		}

		c.Set("Content-Type", "text/html")
		return pages.PortalPortfolio(items, name, settings).Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalContact handles GET /portal/contact — quote request form.
func PortalContact(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)
		c.Set("Content-Type", "text/html")
		return pages.PortalContact(name, settings, false, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalContactSubmit handles POST /portal/contact — saves an inbound quote request.
// Rate limiting is applied at the route level in main.go.
func PortalContactSubmit(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		reqName := strings.TrimSpace(c.FormValue("name"))
		reqEmail := strings.TrimSpace(c.FormValue("email"))

		if reqName == "" || reqEmail == "" {
			c.Set("Content-Type", "text/html")
			return pages.PortalContact(name, settings, false, "Name and email are required.").Render(c.Context(), c.Response().BodyWriter())
		}

		if !validateEmail(reqEmail) {
			c.Set("Content-Type", "text/html")
			return pages.PortalContact(name, settings, false, "Please enter a valid email address.").Render(c.Context(), c.Response().BodyWriter())
		}

		desc := c.FormValue("description")
		if len(desc) > 2000 {
			c.Set("Content-Type", "text/html")
			return pages.PortalContact(name, settings, false, "Description must be 2000 characters or less.").Render(c.Context(), c.Response().BodyWriter())
		}

		req := &models.QuoteRequest{
			Name:        reqName,
			Email:       reqEmail,
			Phone:       strings.TrimSpace(c.FormValue("phone")),
			Company:     strings.TrimSpace(c.FormValue("company")),
			ProjectType: c.FormValue("project_type"),
			Description: desc,
			BudgetRange: c.FormValue("budget_range"),
		}

		if err := models.CreateQuoteRequest(db, req); err != nil {
			log.Printf("portal contact submit: failed to create quote request: %v", err)
			c.Set("Content-Type", "text/html")
			return pages.PortalContact(name, settings, false, "Something went wrong. Please try again.").Render(c.Context(), c.Response().BodyWriter())
		}

		models.LogActivityWithContext(db, "quote_request", req.ID, "created",
			"New quote request from "+sanitizeLogInput(reqName)+" ("+sanitizeLogInput(reqEmail)+")",
			c.IP(), c.Get("User-Agent"))

		c.Set("Content-Type", "text/html")
		return pages.PortalContact(name, settings, true, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalLogin handles GET /portal/login — magic link email form.
func PortalLogin(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)
		c.Set("Content-Type", "text/html")
		return pages.PortalLogin(name, settings, false, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalLoginSubmit handles POST /portal/login.
// Looks up the customer by email, generates a magic token, and logs the plain
// token (in production this would be emailed). Always shows "check your email"
// to avoid email enumeration.
func PortalLoginSubmit(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		email := strings.TrimSpace(c.FormValue("email"))
		if !validateEmail(email) || email == "" {
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "Please enter a valid email address.").Render(c.Context(), c.Response().BodyWriter())
		}

		// Attempt to find the customer — fail silently to prevent enumeration.
		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("portal login: failed to load customers: %v", err)
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, true, "").Render(c.Context(), c.Response().BodyWriter())
		}

		var matchedCustomer *models.Customer
		for _, cu := range customers {
			if strings.EqualFold(cu.Email, email) {
				cu := cu
				matchedCustomer = &cu
				break
			}
		}

		if matchedCustomer != nil {
			plain, hashed, err := portal.GenerateMagicToken()
			if err != nil {
				log.Printf("portal login: failed to generate token: %v", err)
			} else {
				expiresAt := time.Now().Add(15 * time.Minute)
				_, err = models.CreateClientToken(db, matchedCustomer.ID, hashed, expiresAt)
				if err != nil {
					log.Printf("portal login: failed to store token: %v", err)
				} else {
					// In production, send plain token via email.
					// For now, log it so the developer can test the flow.
					log.Printf("portal magic link for %s: /portal/verify/%s (expires %s)",
						email, plain, expiresAt.Format(time.RFC3339))
				}
			}
		}

		// Always render the "check your email" state.
		c.Set("Content-Type", "text/html")
		return pages.PortalLogin(name, settings, true, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

// PortalVerifyToken handles GET /portal/verify/:token — validates a magic link,
// sets the client_token cookie, and redirects to the dashboard.
func PortalVerifyToken(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		plainToken := c.Params("token")
		if plainToken == "" {
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "Invalid sign-in link.").Render(c.Context(), c.Response().BodyWriter())
		}

		// Clean up expired tokens opportunistically.
		_ = models.DeleteExpiredClientTokens(db)

		hash := portal.HashToken(plainToken)
		token, err := models.GetClientTokenByHash(db, hash)
		if err != nil {
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "This sign-in link is invalid or has already been used.").Render(c.Context(), c.Response().BodyWriter())
		}

		if time.Now().After(token.ExpiresAt) {
			_ = models.DeleteClientToken(db, token.ID)
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "This sign-in link has expired. Please request a new one.").Render(c.Context(), c.Response().BodyWriter())
		}

		// Consume the token — each magic link is single-use.
		_ = models.DeleteClientToken(db, token.ID)

		// Issue a new long-lived session token stored as a cookie.
		sessionPlain, sessionHash, err := portal.GenerateMagicToken()
		if err != nil {
			log.Printf("portal verify: failed to generate session token: %v", err)
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "Sign-in failed. Please try again.").Render(c.Context(), c.Response().BodyWriter())
		}

		sessionExpiry := time.Now().Add(7 * 24 * time.Hour)
		_, err = models.CreateClientToken(db, token.CustomerID, sessionHash, sessionExpiry)
		if err != nil {
			log.Printf("portal verify: failed to store session token: %v", err)
			c.Set("Content-Type", "text/html")
			return pages.PortalLogin(name, settings, false, "Sign-in failed. Please try again.").Render(c.Context(), c.Response().BodyWriter())
		}

		c.Cookie(&fiber.Cookie{
			Name:     "client_token",
			Value:    sessionPlain,
			Expires:  sessionExpiry,
			HTTPOnly: true,
			SameSite: "Lax",
			Path:     "/portal",
		})

		return c.Redirect("/portal/dashboard")
	}
}

// PortalDashboard handles GET /portal/dashboard — authenticated client self-service.
func PortalDashboard(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name, settings := portalSettings(db)

		customerID, ok := c.Locals("customer_id").(int)
		if !ok || customerID == 0 {
			return c.Redirect("/portal/login")
		}

		customer, err := models.GetCustomerByID(db, customerID)
		if err != nil {
			log.Printf("portal dashboard: customer %d not found: %v", customerID, err)
			return c.Redirect("/portal/login")
		}

		// Load all sites belonging to this customer.
		allSites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("portal dashboard: failed to load sites: %v", err)
			allSites = nil
		}
		var customerSites []models.Site
		for _, s := range allSites {
			if s.CustomerID.Valid && int(s.CustomerID.Int64) == customerID {
				customerSites = append(customerSites, s)
			}
		}

		payments, err := models.GetPaymentsByCustomerID(db, customerID)
		if err != nil {
			log.Printf("portal dashboard: failed to load payments: %v", err)
		}

		subscriptions, err := models.GetSubscriptionsByCustomerID(db, customerID)
		if err != nil {
			log.Printf("portal dashboard: failed to load subscriptions: %v", err)
		}

		c.Set("Content-Type", "text/html")
		return pages.PortalDashboard(customer, customerSites, payments, subscriptions, name, settings).Render(c.Context(), c.Response().BodyWriter())
	}
}

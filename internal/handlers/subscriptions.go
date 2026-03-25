package handlers

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"

	"ezweb/internal/models"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

// ListSubscriptions handles GET /subscriptions.
func ListSubscriptions(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}

		total, _ := models.CountSubscriptions(db)
		offset := (page - 1) * perPage

		subscriptions, err := models.GetSubscriptionsPaginated(db, perPage, offset)
		if err != nil {
			log.Printf("failed to list subscriptions: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load subscriptions")
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers for subscription form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load customers")
		}

		sites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("failed to load sites for subscription form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load sites")
		}

		c.Set("Content-Type", "text/html")
		return pages.Subscriptions(subscriptions, customers, sites, page, total, perPage).Render(c.Context(), c.Response().BodyWriter())
	}
}

// CreateSubscription handles POST /subscriptions.
func CreateSubscription(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		customerID, err := strconv.Atoi(c.FormValue("customer_id"))
		if err != nil || customerID == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid customer is required")
		}

		amount, err := strconv.ParseFloat(c.FormValue("amount"), 64)
		if err != nil || amount <= 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid amount is required")
		}

		nextDueDate := c.FormValue("next_due_date")
		if nextDueDate == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Next due date is required")
		}

		billingCycle := c.FormValue("billing_cycle")
		if billingCycle != "monthly" && billingCycle != "yearly" {
			billingCycle = "monthly"
		}

		autoGenerate := c.FormValue("auto_generate_invoice") == "1"

		sub := &models.Subscription{
			CustomerID:          customerID,
			Amount:              amount,
			BillingCycle:        billingCycle,
			NextDueDate:         nextDueDate,
			Status:              "active",
			AutoGenerateInvoice: autoGenerate,
		}

		if siteIDStr := c.FormValue("site_id"); siteIDStr != "" {
			siteID, err := strconv.Atoi(siteIDStr)
			if err == nil && siteID > 0 {
				sub.SiteID = sql.NullInt64{Int64: int64(siteID), Valid: true}
			}
		}

		if err := models.CreateSubscription(db, sub); err != nil {
			log.Printf("failed to create subscription: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create subscription")
		}

		models.LogActivityWithContext(db, "subscription", sub.ID, "created",
			fmt.Sprintf("Created subscription for customer %d: $%.2f %s", customerID, amount, billingCycle),
			c.IP(), c.Get("User-Agent"))

		// Reload to get JOIN fields.
		created, err := models.GetSubscriptionByID(db, sub.ID)
		if err != nil {
			log.Printf("failed to reload subscription %d: %v", sub.ID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Subscription created but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SubscriptionRow(*created).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/subscriptions")
	}
}

// PauseSubscription handles POST /subscriptions/:id/pause.
func PauseSubscription(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid subscription ID")
		}

		if _, err := models.GetSubscriptionByID(db, id); err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Subscription not found")
		}

		if err := models.PauseSubscription(db, id); err != nil {
			log.Printf("failed to pause subscription %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to pause subscription")
		}

		models.LogActivityWithContext(db, "subscription", id, "paused", "Subscription paused", c.IP(), c.Get("User-Agent"))

		updated, err := models.GetSubscriptionByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Paused but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SubscriptionRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/subscriptions")
	}
}

// ResumeSubscription handles POST /subscriptions/:id/resume — re-activates a paused subscription.
func ResumeSubscription(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid subscription ID")
		}

		sub, err := models.GetSubscriptionByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Subscription not found")
		}

		if sub.Status != "paused" {
			return c.Status(fiber.StatusBadRequest).SendString("Only paused subscriptions can be resumed")
		}

		sub.Status = "active"
		if err := models.UpdateSubscription(db, sub); err != nil {
			log.Printf("failed to resume subscription %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to resume subscription")
		}

		models.LogActivityWithContext(db, "subscription", id, "resumed", "Subscription resumed", c.IP(), c.Get("User-Agent"))

		updated, err := models.GetSubscriptionByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Resumed but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SubscriptionRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/subscriptions")
	}
}

// CancelSubscription handles POST /subscriptions/:id/cancel.
func CancelSubscription(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid subscription ID")
		}

		if _, err := models.GetSubscriptionByID(db, id); err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Subscription not found")
		}

		if err := models.CancelSubscription(db, id); err != nil {
			log.Printf("failed to cancel subscription %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to cancel subscription")
		}

		models.LogActivityWithContext(db, "subscription", id, "cancelled", "Subscription cancelled", c.IP(), c.Get("User-Agent"))

		updated, err := models.GetSubscriptionByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Cancelled but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.SubscriptionRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/subscriptions")
	}
}

// DeleteSubscription handles DELETE /subscriptions/:id.
func DeleteSubscription(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid subscription ID")
		}

		if _, err := models.GetSubscriptionByID(db, id); err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Subscription not found")
		}

		if err := models.DeleteSubscription(db, id); err != nil {
			log.Printf("failed to delete subscription %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete subscription")
		}

		models.LogActivityWithContext(db, "subscription", id, "deleted", "Deleted subscription", c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/subscriptions")
	}
}

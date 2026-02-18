package handlers

import (
	"database/sql"
	"log"
	"strconv"

	"ezweb/internal/models"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

func ListPayments(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		payments, err := models.GetAllPayments(db)
		if err != nil {
			log.Printf("failed to list payments: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load payments")
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to list customers for payment form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load customers")
		}

		sites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("failed to list sites for payment form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load sites")
		}

		c.Set("Content-Type", "text/html")
		return pages.Payments(payments, customers, sites).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreatePayment(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		customerID, err := strconv.Atoi(c.FormValue("customer_id"))
		if err != nil || customerID == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid customer is required")
		}

		amount, err := strconv.ParseFloat(c.FormValue("amount"), 64)
		if err != nil || amount <= 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid amount is required")
		}

		dueDate := c.FormValue("due_date")
		if dueDate == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Due date is required")
		}

		p := &models.Payment{
			CustomerID: customerID,
			Amount:     amount,
			DueDate:    dueDate,
			Notes:      c.FormValue("notes"),
		}

		// Handle optional site_id
		if siteIDStr := c.FormValue("site_id"); siteIDStr != "" {
			siteID, err := strconv.Atoi(siteIDStr)
			if err == nil && siteID > 0 {
				p.SiteID = sql.NullInt64{Int64: int64(siteID), Valid: true}
			}
		}

		if err := models.CreatePayment(db, p); err != nil {
			log.Printf("failed to create payment: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create payment")
		}

		models.LogActivity(db, "payment", p.ID, "created", "Created payment of $"+c.FormValue("amount")+" due "+dueDate)

		// Reload from DB to get computed status and JOIN fields
		payment, err := models.GetPaymentByID(db, p.ID)
		if err != nil {
			log.Printf("failed to reload payment %d: %v", p.ID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Payment created but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.PaymentRow(*payment).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/payments")
	}
}

func EditPaymentForm(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid payment ID")
		}

		payment, err := models.GetPaymentByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Payment not found")
		}

		customers, err := models.GetAllCustomers(db)
		if err != nil {
			log.Printf("failed to load customers for edit form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load customers")
		}

		sites, err := models.GetAllSites(db)
		if err != nil {
			log.Printf("failed to load sites for edit form: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load sites")
		}

		c.Set("Content-Type", "text/html")
		return partials.PaymentEditRow(*payment, customers, sites).Render(c.Context(), c.Response().BodyWriter())
	}
}

func UpdatePayment(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid payment ID")
		}

		customerID, err := strconv.Atoi(c.FormValue("customer_id"))
		if err != nil || customerID == 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid customer is required")
		}

		amount, err := strconv.ParseFloat(c.FormValue("amount"), 64)
		if err != nil || amount <= 0 {
			return c.Status(fiber.StatusBadRequest).SendString("Valid amount is required")
		}

		dueDate := c.FormValue("due_date")
		if dueDate == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Due date is required")
		}

		p := &models.Payment{
			ID:         id,
			CustomerID: customerID,
			Amount:     amount,
			DueDate:    dueDate,
			Notes:      c.FormValue("notes"),
		}

		if siteIDStr := c.FormValue("site_id"); siteIDStr != "" {
			siteID, err := strconv.Atoi(siteIDStr)
			if err == nil && siteID > 0 {
				p.SiteID = sql.NullInt64{Int64: int64(siteID), Valid: true}
			}
		}

		if err := models.UpdatePayment(db, p); err != nil {
			log.Printf("failed to update payment %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update payment")
		}

		models.LogActivity(db, "payment", id, "updated", "Updated payment of $"+c.FormValue("amount"))

		payment, err := models.GetPaymentByID(db, id)
		if err != nil {
			log.Printf("failed to reload payment %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Payment updated but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.PaymentRow(*payment).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/payments")
	}
}

func MarkPaid(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid payment ID")
		}

		if err := models.MarkPaymentPaid(db, id); err != nil {
			log.Printf("failed to mark payment %d as paid: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to mark payment as paid")
		}

		models.LogActivity(db, "payment", id, "paid", "Marked payment as paid")

		payment, err := models.GetPaymentByID(db, id)
		if err != nil {
			log.Printf("failed to reload payment %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Marked as paid but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.PaymentRow(*payment).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/payments")
	}
}

func CancelEditPayment(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid payment ID")
		}

		payment, err := models.GetPaymentByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Payment not found")
		}

		c.Set("Content-Type", "text/html")
		return partials.PaymentRow(*payment).Render(c.Context(), c.Response().BodyWriter())
	}
}

func DeletePayment(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid payment ID")
		}

		if err := models.DeletePayment(db, id); err != nil {
			log.Printf("failed to delete payment %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete payment")
		}

		models.LogActivity(db, "payment", id, "deleted", "Deleted payment")

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/payments")
	}
}

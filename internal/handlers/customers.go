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

const perPage = 25

func ListCustomers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		page, _ := strconv.Atoi(c.Query("page", "1"))
		if page < 1 {
			page = 1
		}

		total, _ := models.CountCustomers(db)
		offset := (page - 1) * perPage

		customers, err := models.GetCustomersPaginated(db, perPage, offset)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load customers")
		}

		c.Set("Content-Type", "text/html")
		return pages.Customers(customers, "", page, total, perPage).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateCustomer(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		customer := &models.Customer{
			Name:    c.FormValue("name"),
			Email:   c.FormValue("email"),
			Phone:   c.FormValue("phone"),
			Company: c.FormValue("company"),
		}

		if customer.Name == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Name is required")
		}
		if !validateEmail(customer.Email) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid email format")
		}
		if !validatePhone(customer.Phone) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid phone format")
		}

		if err := models.CreateCustomer(db, customer); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create customer")
		}

		models.LogActivityWithContext(db, "customer", customer.ID, "created", "Added customer "+customer.Name, c.IP(), c.Get("User-Agent"))

		// Reload the customer to get the full record with timestamps
		created, err := models.GetCustomerByID(db, customer.ID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to reload customer")
		}

		// HTMX request: return just the new row
		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.CustomerRow(*created).Render(c.Context(), c.Response().BodyWriter())
		}

		return c.Redirect("/customers")
	}
}

func EditCustomerForm(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid customer ID")
		}

		customer, err := models.GetCustomerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Customer not found")
		}

		c.Set("Content-Type", "text/html")
		return partials.CustomerEditRow(*customer).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CancelEditCustomer(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid customer ID")
		}

		customer, err := models.GetCustomerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Customer not found")
		}

		c.Set("Content-Type", "text/html")
		return partials.CustomerRow(*customer).Render(c.Context(), c.Response().BodyWriter())
	}
}

func UpdateCustomer(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid customer ID")
		}

		customer := &models.Customer{
			ID:      id,
			Name:    c.FormValue("name"),
			Email:   c.FormValue("email"),
			Phone:   c.FormValue("phone"),
			Company: c.FormValue("company"),
		}

		if customer.Name == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Name is required")
		}
		if !validateEmail(customer.Email) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid email format")
		}
		if !validatePhone(customer.Phone) {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid phone format")
		}

		if err := models.UpdateCustomer(db, customer); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update customer")
		}

		models.LogActivityWithContext(db, "customer", id, "updated", "Updated customer "+customer.Name, c.IP(), c.Get("User-Agent"))

		// Reload to get updated timestamps
		updated, err := models.GetCustomerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to reload customer")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.CustomerRow(*updated).Render(c.Context(), c.Response().BodyWriter())
		}

		return c.Redirect("/customers")
	}
}

func DeleteCustomer(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid customer ID")
		}

		customer, err := models.GetCustomerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Customer not found")
		}

		tx, err := db.Begin()
		if err != nil {
			log.Printf("failed to begin transaction for customer delete %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}
		defer tx.Rollback() //nolint:errcheck

		if _, err := tx.Exec("UPDATE sites SET customer_id = NULL WHERE customer_id = ?", id); err != nil {
			log.Printf("failed to detach sites from customer %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}

		if _, err := tx.Exec("DELETE FROM payments WHERE customer_id = ?", id); err != nil {
			log.Printf("failed to remove payments for customer %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}

		if _, err := tx.Exec("DELETE FROM customers WHERE id = ?", id); err != nil {
			log.Printf("failed to delete customer %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}

		if err := tx.Commit(); err != nil {
			log.Printf("failed to commit customer delete %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}

		models.LogActivityWithContext(db, "customer", id, "deleted", "Deleted customer "+customer.Name, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}

		return c.Redirect("/customers")
	}
}

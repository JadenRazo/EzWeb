package handlers

import (
	"database/sql"
	"strconv"

	"ezweb/internal/models"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

func ListCustomers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		customers, err := models.GetAllCustomers(db)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load customers")
		}

		c.Set("Content-Type", "text/html")
		return pages.Customers(customers, "").Render(c.Context(), c.Response().BodyWriter())
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

		if err := models.CreateCustomer(db, customer); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create customer")
		}

		models.LogActivity(db, "customer", customer.ID, "created", "Added customer "+customer.Name)

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

		if err := models.UpdateCustomer(db, customer); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update customer")
		}

		models.LogActivity(db, "customer", id, "updated", "Updated customer "+customer.Name)

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

		if err := models.DeleteCustomer(db, id); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete customer")
		}

		models.LogActivity(db, "customer", id, "deleted", "Deleted customer "+customer.Name)

		// HTMX: return empty string so the row is removed
		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}

		return c.Redirect("/customers")
	}
}

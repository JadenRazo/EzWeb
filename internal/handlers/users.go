package handlers

import (
	"database/sql"
	"log"
	"strconv"

	"ezweb/internal/auth"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func ListUsers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		users, err := models.GetAllUsers(db)
		if err != nil {
			log.Printf("failed to list users: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load users")
		}

		c.Set("Content-Type", "text/html")
		return pages.Users(users).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateUser(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		role := c.FormValue("role", "viewer")

		if username == "" || password == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Username and password are required")
		}

		if len(password) < 8 {
			return c.Status(fiber.StatusBadRequest).SendString("Password must be at least 8 characters")
		}

		if role != "admin" && role != "viewer" {
			role = "viewer"
		}

		hash, err := auth.HashPassword(password)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to hash password")
		}

		if err := models.CreateUserWithRole(db, username, hash, role); err != nil {
			log.Printf("failed to create user: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create user")
		}

		models.LogActivityWithContext(db, "user", 0, "created", "Created user "+username, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/users")
			return c.SendString("")
		}
		return c.Redirect("/users")
	}
}

func DeleteUserHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid user ID")
		}

		// Prevent deleting yourself
		currentUserID, _ := c.Locals("user_id").(int)
		if id == currentUserID {
			return c.Status(fiber.StatusBadRequest).SendString("Cannot delete your own account")
		}

		if err := models.DeleteUser(db, id); err != nil {
			log.Printf("failed to delete user %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete user")
		}

		models.LogActivityWithContext(db, "user", id, "deleted", "Deleted user", c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/users")
	}
}

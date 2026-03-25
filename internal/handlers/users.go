package handlers

import (
	"database/sql"
	"log"
	"regexp"
	"strconv"

	"ezweb/internal/auth"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

var validUsername = regexp.MustCompile(`^[a-zA-Z0-9._-]{3,50}$`)

func ListUsers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		users, err := models.GetAllUsers(db)
		if err != nil {
			log.Printf("failed to list users: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load users")
		}

		currentUsername, _ := c.Locals("username").(string)
		c.Set("Content-Type", "text/html")
		return pages.Users(users, currentUsername).Render(c.Context(), c.Response().BodyWriter())
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

		if !validUsername.MatchString(username) {
			return c.Status(fiber.StatusBadRequest).SendString("Username must be 3-50 characters and may only contain letters, numbers, underscores, hyphens, and dots")
		}

		if err := auth.ValidatePasswordStrength(password); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
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

// ChangePassword handles PUT /users/:id/password.
// When a user changes their own password, current_password must match the stored hash.
// Admins changing another user's password bypass the current_password check.
func ChangePassword(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targetID, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid user ID")
		}

		currentUserID, _ := c.Locals("user_id").(int)
		isSelf := targetID == currentUserID

		newPassword := c.FormValue("new_password")
		if err := auth.ValidatePasswordStrength(newPassword); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString(err.Error())
		}

		target, err := models.GetUserByID(db, targetID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		if isSelf {
			currentPassword := c.FormValue("current_password")
			if currentPassword == "" {
				return c.Status(fiber.StatusBadRequest).SendString("Current password is required")
			}
			if !auth.CheckPassword(target.Password, currentPassword) {
				return c.Status(fiber.StatusUnauthorized).SendString("Current password is incorrect")
			}
		}

		hash, err := auth.HashPassword(newPassword)
		if err != nil {
			log.Printf("failed to hash password for user %d: %v", targetID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to hash password")
		}

		if err := models.UpdateUserPassword(db, targetID, hash); err != nil {
			log.Printf("failed to update password for user %d: %v", targetID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update password")
		}

		models.LogActivityWithContext(db, "user", targetID, "password_changed",
			"Password changed for user "+target.Username, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/users")
			return c.SendString("")
		}
		return c.Redirect("/users")
	}
}

// UpdateUserRoleHandler handles PUT /users/:id/role.
// Admins may change any user's role except their own, preventing self-lockout.
func UpdateUserRoleHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		targetID, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid user ID")
		}

		currentUserID, _ := c.Locals("user_id").(int)
		if targetID == currentUserID {
			return c.Status(fiber.StatusBadRequest).SendString("You cannot change your own role")
		}

		role := c.FormValue("role")
		if role != "admin" && role != "viewer" {
			return c.Status(fiber.StatusBadRequest).SendString("Role must be 'admin' or 'viewer'")
		}

		target, err := models.GetUserByID(db, targetID)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("User not found")
		}

		if err := models.UpdateUserRole(db, targetID, role); err != nil {
			log.Printf("failed to update role for user %d: %v", targetID, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update role")
		}

		models.LogActivityWithContext(db, "user", targetID, "role_changed",
			"Role changed to "+role+" for user "+target.Username, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/users")
			return c.SendString("")
		}
		return c.Redirect("/users")
	}
}

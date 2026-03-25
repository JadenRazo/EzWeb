package auth

import (
	"database/sql"

	"github.com/gofiber/fiber/v2"
)

func AuthMiddleware(secret string, db ...*sql.DB) fiber.Handler {
	var database *sql.DB
	if len(db) > 0 {
		database = db[0]
	}
	return func(c *fiber.Ctx) error {
		tokenStr := c.Cookies("token")
		if tokenStr == "" {
			return c.Redirect("/login")
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			c.ClearCookie("token")
			return c.Redirect("/login")
		}

		// Check token revocation blocklist
		if database != nil && claims.ID != "" && IsRevoked(database, claims.ID) {
			c.ClearCookie("token")
			return c.Redirect("/login")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("username", claims.Username)
		c.Locals("role", claims.Role)
		c.Locals("token_claims", claims)
		return c.Next()
	}
}

// AdminOnly is a middleware that restricts access to admin-role users only.
// It must be used after AuthMiddleware so that role is already set in locals.
func AdminOnly() fiber.Handler {
	return func(c *fiber.Ctx) error {
		role, _ := c.Locals("role").(string)
		if role != "admin" {
			return c.Status(fiber.StatusForbidden).SendString("Admin access required")
		}
		return c.Next()
	}
}

// WriteProtect restricts mutating HTTP methods (POST, PUT, DELETE, PATCH) to
// admin-role users. Read-only methods (GET, HEAD, OPTIONS) pass through for
// any authenticated user. Must be placed after AuthMiddleware.
func WriteProtect() fiber.Handler {
	return func(c *fiber.Ctx) error {
		switch c.Method() {
		case "POST", "PUT", "DELETE", "PATCH":
			role, _ := c.Locals("role").(string)
			if role != "admin" {
				return c.Status(fiber.StatusForbidden).SendString("Admin access required for this action")
			}
		}
		return c.Next()
	}
}

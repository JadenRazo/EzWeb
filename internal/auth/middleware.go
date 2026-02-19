package auth

import (
	"github.com/gofiber/fiber/v2"
)

func AuthMiddleware(secret string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		tokenStr := c.Cookies("token")
		if tokenStr == "" {
			return c.Redirect("/login")
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			// Clear the invalid cookie
			c.ClearCookie("token")
			return c.Redirect("/login")
		}

		c.Locals("user_id", claims.UserID)
		c.Locals("username", claims.Username)
		c.Locals("role", claims.Role)
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

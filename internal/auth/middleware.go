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
		return c.Next()
	}
}

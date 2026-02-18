package handlers

import (
	"database/sql"
	"time"

	"ezweb/internal/auth"
	"ezweb/internal/config"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func LoginPage(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/html")
	return pages.Login("").Render(c.Context(), c.Response().BodyWriter())
}

func LoginPost(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")

		user, err := models.GetUserByUsername(db, username)
		if err != nil || !auth.CheckPassword(user.Password, password) {
			c.Set("Content-Type", "text/html")
			return pages.Login("Invalid username or password").Render(c.Context(), c.Response().BodyWriter())
		}

		token, err := auth.GenerateToken(user.ID, user.Username, cfg.JWTSecret)
		if err != nil {
			c.Set("Content-Type", "text/html")
			return pages.Login("Internal server error").Render(c.Context(), c.Response().BodyWriter())
		}

		c.Cookie(&fiber.Cookie{
			Name:     "token",
			Value:    token,
			HTTPOnly: true,
			Secure:   cfg.SecureCookies,
			SameSite: "Lax",
			Expires:  time.Now().Add(24 * time.Hour),
			Path:     "/",
		})

		return c.Redirect("/dashboard")
	}
}

func Logout(c *fiber.Ctx) error {
	c.Cookie(&fiber.Cookie{
		Name:     "token",
		Value:    "",
		HTTPOnly: true,
		Expires:  time.Now().Add(-1 * time.Hour),
		Path:     "/",
	})
	return c.Redirect("/login")
}

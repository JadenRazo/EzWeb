package handlers

import (
	"database/sql"
	"log"
	"strings"
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

func LoginPost(db *sql.DB, cfg *config.Config, lockout *auth.LockoutTracker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		clientIP := c.IP()

		if lockout.IsLocked(clientIP) {
			c.Set("Content-Type", "text/html")
			return pages.Login("Too many failed attempts. Please try again later.").Render(c.Context(), c.Response().BodyWriter())
		}

		safeUser := strings.ReplaceAll(strings.ReplaceAll(username, "\n", ""), "\r", "")

		user, err := models.GetUserByUsername(db, username)
		if err != nil || !auth.CheckPassword(user.Password, password) {
			lockout.RecordFailure(clientIP)
			log.Printf("failed login attempt for user %q from %s", safeUser, clientIP)
			c.Set("Content-Type", "text/html")
			return pages.Login("Invalid username or password").Render(c.Context(), c.Response().BodyWriter())
		}

		lockout.Reset(clientIP)
		safeDBUser := strings.ReplaceAll(strings.ReplaceAll(user.Username, "\n", ""), "\r", "")
		log.Printf("successful login for user %q from %s", safeDBUser, clientIP)

		token, err := auth.GenerateToken(user.ID, user.Username, user.Role, cfg.JWTSecret, cfg.JWTExpiryHours)
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
			Expires:  time.Now().Add(time.Duration(cfg.JWTExpiryHours) * time.Hour),
			Path:     "/",
		})

		return c.Redirect("/dashboard")
	}
}

// Logout returns a handler that clears the session cookie with the same
// attributes used when setting it, so the browser actually removes the cookie.
func Logout(cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Cookie(&fiber.Cookie{
			Name:     "token",
			Value:    "",
			HTTPOnly: true,
			Secure:   cfg.SecureCookies,
			SameSite: "Lax",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/",
		})
		return c.Redirect("/login")
	}
}

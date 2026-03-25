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

func LoginPost(db *sql.DB, cfg *config.Config, lockout *auth.LockoutTracker, userLockout *auth.LockoutTracker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username := c.FormValue("username")
		password := c.FormValue("password")
		clientIP := c.IP()

		safeUser := strings.ReplaceAll(strings.ReplaceAll(username, "\n", ""), "\r", "")

		if lockout.IsLocked(clientIP) || userLockout.IsLocked(strings.ToLower(username)) {
			c.Set("Content-Type", "text/html")
			return pages.Login("Too many failed attempts. Please try again later.").Render(c.Context(), c.Response().BodyWriter())
		}

		user, err := models.GetUserByUsername(db, username)
		if err != nil || !auth.CheckPassword(user.Password, password) {
			lockout.RecordFailure(clientIP)
			userLockout.RecordFailure(strings.ToLower(username))
			log.Printf("failed login attempt for user %q from %s", safeUser, clientIP)
			models.LogActivityWithContext(db, "auth", 0, "login_failed", "Failed login for user "+safeUser, clientIP, c.Get("User-Agent"))
			c.Set("Content-Type", "text/html")
			return pages.Login("Invalid username or password").Render(c.Context(), c.Response().BodyWriter())
		}

		lockout.Reset(clientIP)
		userLockout.Reset(strings.ToLower(username))
		safeDBUser := strings.ReplaceAll(strings.ReplaceAll(user.Username, "\n", ""), "\r", "")
		log.Printf("successful login for user %q from %s", safeDBUser, clientIP)

		// If 2FA is enabled, redirect to TOTP verification
		if user.TOTPEnabled {
			pendingToken, err := auth.GeneratePendingToken(user.ID, cfg.JWTSecret)
			if err != nil {
				c.Set("Content-Type", "text/html")
				return pages.Login("Internal server error").Render(c.Context(), c.Response().BodyWriter())
			}
			c.Cookie(&fiber.Cookie{
				Name:     "totp_pending",
				Value:    pendingToken,
				HTTPOnly: true,
				Secure:   cfg.SecureCookies,
				SameSite: "Lax",
				Expires:  time.Now().Add(5 * time.Minute),
				Path:     "/login",
			})
			return c.Redirect("/login/2fa")
		}

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

func TOTPVerifyPage(c *fiber.Ctx) error {
	// Only show this page if user has a pending token
	if c.Cookies("totp_pending") == "" {
		return c.Redirect("/login")
	}
	c.Set("Content-Type", "text/html")
	return pages.TOTPVerify("").Render(c.Context(), c.Response().BodyWriter())
}

func TOTPVerifyPost(db *sql.DB, cfg *config.Config, lockout *auth.LockoutTracker) fiber.Handler {
	return func(c *fiber.Ctx) error {
		pendingToken := c.Cookies("totp_pending")
		if pendingToken == "" {
			return c.Redirect("/login")
		}

		userID, err := auth.ValidatePendingToken(pendingToken, cfg.JWTSecret)
		if err != nil {
			// Expired or invalid pending token — restart login
			c.Cookie(&fiber.Cookie{
				Name:     "totp_pending",
				Value:    "",
				HTTPOnly: true,
				Secure:   cfg.SecureCookies,
				SameSite: "Lax",
				Expires:  time.Now().Add(-1 * time.Hour),
				Path:     "/login",
			})
			return c.Redirect("/login")
		}

		clientIP := c.IP()
		lockoutKey := "totp:" + clientIP

		if lockout.IsLocked(lockoutKey) {
			c.Set("Content-Type", "text/html")
			return pages.TOTPVerify("Too many failed attempts. Please try again later.").Render(c.Context(), c.Response().BodyWriter())
		}

		code := c.FormValue("code")

		user, err := models.GetUserByID(db, userID)
		if err != nil {
			return c.Redirect("/login")
		}

		// Check replay
		used, err := models.IsTOTPCodeUsed(db, userID, code)
		if err != nil {
			c.Set("Content-Type", "text/html")
			return pages.TOTPVerify("Internal server error").Render(c.Context(), c.Response().BodyWriter())
		}
		if used {
			lockout.RecordFailure(lockoutKey)
			c.Set("Content-Type", "text/html")
			return pages.TOTPVerify("Code already used. Wait for a new code.").Render(c.Context(), c.Response().BodyWriter())
		}

		if !auth.ValidateTOTPCode(code, user.TOTPSecret) {
			lockout.RecordFailure(lockoutKey)
			log.Printf("failed 2FA attempt for user %d from %s", userID, clientIP)
			c.Set("Content-Type", "text/html")
			return pages.TOTPVerify("Invalid verification code").Render(c.Context(), c.Response().BodyWriter())
		}

		// Record code to prevent replay
		models.RecordTOTPCodeUsed(db, userID, code)
		lockout.Reset(lockoutKey)

		// Clear pending cookie
		c.Cookie(&fiber.Cookie{
			Name:     "totp_pending",
			Value:    "",
			HTTPOnly: true,
			Secure:   cfg.SecureCookies,
			SameSite: "Lax",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/login",
		})

		// Issue full JWT
		token, err := auth.GenerateToken(user.ID, user.Username, user.Role, cfg.JWTSecret, cfg.JWTExpiryHours)
		if err != nil {
			c.Set("Content-Type", "text/html")
			return pages.TOTPVerify("Internal server error").Render(c.Context(), c.Response().BodyWriter())
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

// Logout returns a handler that revokes the current JWT and clears the cookie.
func Logout(cfg *config.Config, db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Parse and revoke the token so it cannot be reused even if stolen
		tokenStr := c.Cookies("token")
		if tokenStr != "" {
			if claims, err := auth.ValidateToken(tokenStr, cfg.JWTSecret); err == nil && claims.ID != "" {
				expiresAt := time.Now().Add(time.Duration(cfg.JWTExpiryHours) * time.Hour)
				if claims.ExpiresAt != nil {
					expiresAt = claims.ExpiresAt.Time
				}
				if err := auth.RevokeToken(db, claims.ID, expiresAt); err != nil {
					log.Printf("failed to revoke token on logout: %v", err)
				}
			}
		}

		c.Cookie(&fiber.Cookie{
			Name:     "token",
			Value:    "",
			HTTPOnly: true,
			Secure:   cfg.SecureCookies,
			SameSite: "Lax",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/",
		})
		c.Cookie(&fiber.Cookie{
			Name:     "totp_pending",
			Value:    "",
			HTTPOnly: true,
			Secure:   cfg.SecureCookies,
			SameSite: "Lax",
			Expires:  time.Now().Add(-1 * time.Hour),
			Path:     "/login",
		})
		return c.Redirect("/login")
	}
}

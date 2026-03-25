package handlers

import (
	"database/sql"

	"ezweb/internal/auth"
	"ezweb/internal/config"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func TOTPSetupPage(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(int)
		user, err := models.GetUserByID(db, userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user")
		}

		if user.TOTPEnabled {
			c.Set("Content-Type", "text/html")
			return pages.TOTPSetup("", "", "", true, "").Render(c.Context(), c.Response().BodyWriter())
		}

		key, qrDataURI, err := auth.GenerateTOTPSecret(user.Username, cfg.TOTPIssuer)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to generate TOTP secret")
		}

		// Store the secret temporarily — it won't be "enabled" until verified
		if err := models.SetTOTPSecret(db, userID, key.Secret()); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save TOTP secret")
		}

		c.Set("Content-Type", "text/html")
		return pages.TOTPSetup(qrDataURI, key.Secret(), "", false, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

func TOTPEnable(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(int)
		code := c.FormValue("code")

		user, err := models.GetUserByID(db, userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user")
		}

		if user.TOTPSecret == "" {
			return c.Redirect("/settings/2fa")
		}

		if !auth.ValidateTOTPCode(code, user.TOTPSecret) {
			c.Set("Content-Type", "text/html")
			return pages.TOTPSetup("", user.TOTPSecret, "", false, "Invalid code. Please try again.").Render(c.Context(), c.Response().BodyWriter())
		}

		if err := models.EnableTOTP(db, userID); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to enable 2FA")
		}

		c.Set("Content-Type", "text/html")
		return pages.TOTPSetup("", "", "", true, "").Render(c.Context(), c.Response().BodyWriter())
	}
}

func TOTPDisable(db *sql.DB, cfg *config.Config) fiber.Handler {
	return func(c *fiber.Ctx) error {
		userID := c.Locals("user_id").(int)
		code := c.FormValue("code")

		user, err := models.GetUserByID(db, userID)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load user")
		}

		if !user.TOTPEnabled {
			return c.Redirect("/settings/2fa")
		}

		if !auth.ValidateTOTPCode(code, user.TOTPSecret) {
			c.Set("Content-Type", "text/html")
			return pages.TOTPSetup("", "", "", true, "Invalid code. Cannot disable 2FA.").Render(c.Context(), c.Response().BodyWriter())
		}

		if err := models.DisableTOTP(db, userID); err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to disable 2FA")
		}

		return c.Redirect("/settings/2fa")
	}
}

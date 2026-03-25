package handlers

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

// allowedLogoExts is the set of image extensions accepted for logo uploads.
var allowedLogoExts = map[string]bool{
	".jpg":  true,
	".jpeg": true,
	".png":  true,
	".gif":  true,
	".svg":  true,
	".webp": true,
}

const maxLogoBytes = 5 << 20 // 5 MB

// SettingsPage handles GET /settings.
// It loads all business settings and renders the settings page.
func SettingsPage(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		settings, err := models.GetAllSettings(db)
		if err != nil {
			log.Printf("failed to load business settings: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load settings")
		}

		// Pass the success flash through so the template can display it once.
		flash := c.Query("success")

		c.Set("Content-Type", "text/html")
		return pages.Settings(settings, flash).Render(c.Context(), c.Response().BodyWriter())
	}
}

// SaveSettings handles POST /settings.
// It reads every known setting key from the form and bulk-upserts them.
func SaveSettings(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		settings := map[string]string{
			"business_name":      c.FormValue("business_name"),
			"tagline":            c.FormValue("tagline"),
			"email":              c.FormValue("email"),
			"phone":              c.FormValue("phone"),
			"address":            c.FormValue("address"),
			"website_url":        c.FormValue("website_url"),
			"tax_rate":           c.FormValue("tax_rate"),
			"default_currency":   c.FormValue("default_currency"),
			"quote_validity_days": c.FormValue("quote_validity_days"),
			"terms_text":         c.FormValue("terms_text"),
		}

		if err := models.SetSettings(db, settings); err != nil {
			log.Printf("failed to save business settings: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save settings")
		}

		models.LogActivityWithContext(db, "settings", 0, "updated", "Business settings updated", c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/settings?success=1")
			return c.SendString("")
		}

		return c.Redirect("/settings?success=1")
	}
}

// UploadLogo handles POST /settings/logo.
// It accepts a multipart image upload, saves it to static/uploads/, and
// persists the relative path in the logo_path setting.
func UploadLogo(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		fh, err := c.FormFile("logo")
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("No logo file provided")
		}

		if fh.Size > maxLogoBytes {
			return c.Status(fiber.StatusRequestEntityTooLarge).SendString("Logo must be 5 MB or smaller")
		}

		ext := strings.ToLower(filepath.Ext(fh.Filename))
		if !allowedLogoExts[ext] {
			return c.Status(fiber.StatusBadRequest).SendString("Unsupported file type; allowed: jpg, png, gif, svg, webp")
		}

		uploadDir := filepath.Join("static", "uploads")
		if err := os.MkdirAll(uploadDir, 0o755); err != nil {
			log.Printf("failed to create upload directory: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to prepare upload directory")
		}

		filename := fmt.Sprintf("logo_%d%s", time.Now().UnixMilli(), ext)
		destPath := filepath.Join(uploadDir, filename)

		src, err := fh.Open()
		if err != nil {
			log.Printf("failed to open uploaded logo: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to read uploaded file")
		}
		defer src.Close()

		dst, err := os.Create(destPath)
		if err != nil {
			log.Printf("failed to create logo destination file %s: %v", destPath, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to save uploaded file")
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			log.Printf("failed to write logo file %s: %v", destPath, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to write uploaded file")
		}

		// Store path relative to static root so templates can use /static/uploads/logo_…
		relativePath := filepath.Join("uploads", filename)
		if err := models.SetSetting(db, "logo_path", relativePath); err != nil {
			log.Printf("failed to persist logo_path setting: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("File saved but failed to update setting")
		}

		models.LogActivityWithContext(db, "settings", 0, "logo_uploaded", "Logo updated: "+filename, c.IP(), c.Get("User-Agent"))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/settings?success=1")
			return c.SendString("")
		}

		return c.Redirect("/settings?success=1")
	}
}

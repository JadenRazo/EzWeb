package handlers

import (
	"log"
	"strconv"
	"strings"

	"ezweb/internal/backup"
	"ezweb/internal/models"
	"ezweb/views/pages"

	"github.com/gofiber/fiber/v2"
)

func BackupsPage(bm *backup.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		backups, err := bm.ListBackups()
		if err != nil {
			log.Printf("failed to list backups: %v", err)
			backups = nil
		}

		c.Set("Content-Type", "text/html")
		return pages.Backups(backups).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateDatabaseBackup(bm *backup.Manager, dbPath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		bi, err := bm.BackupDatabase(dbPath)
		if err != nil {
			log.Printf("database backup failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Database backup failed")
		}

		log.Printf("database backup created: %s (%s)", bi.Name, backup.FormatSize(bi.Size))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/backups")
			return c.SendString("")
		}
		return c.Redirect("/backups")
	}
}

func CreateSiteBackupHandler(bm *backup.Manager, dbGetter func(int) (*models.Site, error)) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := dbGetter(id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		bi, err := bm.BackupSite(*site)
		if err != nil {
			log.Printf("site backup failed for %s: %v", site.Domain, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Site backup failed")
		}

		log.Printf("site backup created: %s (%s)", bi.Name, backup.FormatSize(bi.Size))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/backups")
			return c.SendString("")
		}
		return c.Redirect("/backups")
	}
}

func CreateFullBackup(bm *backup.Manager, dbPath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		results, err := bm.RunFullBackup(dbPath)
		if err != nil {
			log.Printf("full backup had errors: %v", err)
		}

		log.Printf("full backup completed: %d items", len(results))

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/backups")
			return c.SendString("")
		}
		return c.Redirect("/backups")
	}
}

func DeleteBackup(bm *backup.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name := c.Params("name")
		if name == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Backup name required")
		}

		if err := bm.DeleteBackup(name); err != nil {
			log.Printf("failed to delete backup %s: %v", name, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete backup")
		}

		if c.Get("HX-Request") != "" {
			return c.SendString("")
		}
		return c.Redirect("/backups")
	}
}

func DownloadBackup(bm *backup.Manager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name := c.Params("name")
		if name == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Backup name required")
		}

		backups, err := bm.ListBackups()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to list backups")
		}

		for _, b := range backups {
			if b.Name == name {
				return c.Download(b.Path, b.Name)
			}
		}

		return c.Status(fiber.StatusNotFound).SendString("Backup not found")
	}
}

func RestoreBackup(bm *backup.Manager, dbPath string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		name := c.Params("name")
		if name == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Backup name required")
		}

		if !strings.HasPrefix(name, "ezweb-db-") {
			return c.Status(fiber.StatusBadRequest).SendString("Can only restore database backups")
		}

		if err := bm.RestoreDatabase(name, dbPath); err != nil {
			log.Printf("restore failed for %s: %v", name, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Restore failed: " + err.Error())
		}

		log.Printf("database restored from backup: %s", name)

		if c.Get("HX-Request") != "" {
			c.Set("HX-Redirect", "/backups")
			return c.SendString("")
		}
		return c.Redirect("/backups")
	}
}

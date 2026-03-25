package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"ezweb/internal/docker"
	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

func GetSiteLogs(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		lines := c.QueryInt("lines", 200)
		if lines < 50 {
			lines = 50
		}
		if lines > 2000 {
			lines = 2000
		}

		search := c.Query("search", "")

		var output string

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			output, err = docker.LocalComposeLogs(ctx, site.ComposePath, lines)
			if err != nil {
				log.Printf("failed to get local logs for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to get logs")
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(fiber.StatusBadRequest).SendString("Site has no server assigned")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to get server")
			}

			client, err := sshutil.NewClientWithHostKey(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey)
			if err != nil {
				log.Printf("SSH connection failed for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("SSH connection failed")
			}
			defer client.Close()

			if err := docker.ValidateContainerName(site.ContainerName); err != nil {
				return c.Status(fiber.StatusBadRequest).SendString("Invalid container name")
			}

			output, err = sshutil.RunCommand(client, fmt.Sprintf("cd /opt/ezweb/%s && docker compose logs --tail %d 2>&1", site.ContainerName, lines))
			if err != nil {
				log.Printf("failed to get remote logs for site %d: %v", id, err)
				return c.Status(fiber.StatusInternalServerError).SendString("Failed to get logs")
			}
		}

		// Apply search filter if provided
		if search != "" {
			var filtered []string
			for _, line := range strings.Split(output, "\n") {
				if strings.Contains(strings.ToLower(line), strings.ToLower(search)) {
					filtered = append(filtered, line)
				}
			}
			output = strings.Join(filtered, "\n")
		}

		c.Set("Content-Type", "text/html")
		return partials.LogStream(output).Render(c.Context(), c.Response().BodyWriter())
	}
}

func GetSiteHealth(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		checks, err := models.GetHealthChecksBySiteID(db, id, 20)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to get health checks")
		}

		c.Set("Content-Type", "text/html")
		return partials.HealthChecks(checks).Render(c.Context(), c.Response().BodyWriter())
	}
}

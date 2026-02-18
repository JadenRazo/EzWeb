package handlers

import (
	"context"
	"database/sql"
	"strconv"
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
			return c.Status(400).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(404).SendString("Site not found")
		}

		var output string

		if site.IsLocal && site.ComposePath != "" {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			output, err = docker.LocalComposeLogs(ctx, site.ComposePath, 200)
			if err != nil {
				return c.Status(500).SendString("Failed to get logs: " + err.Error())
			}
		} else {
			if !site.ServerID.Valid {
				return c.Status(400).SendString("Site has no server assigned")
			}

			server, err := models.GetServerByID(db, int(site.ServerID.Int64))
			if err != nil {
				return c.Status(500).SendString("Failed to get server")
			}

			client, err := sshutil.NewClient(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath)
			if err != nil {
				return c.Status(500).SendString("SSH connection failed: " + err.Error())
			}
			defer client.Close()

			output, err = sshutil.RunCommand(client, "cd /opt/ezweb/"+site.ContainerName+" && docker compose logs --tail 200 2>&1")
			if err != nil {
				return c.Status(500).SendString("Failed to get logs: " + err.Error())
			}
		}

		c.Set("Content-Type", "text/html")
		return partials.LogStream(output).Render(c.Context(), c.Response().BodyWriter())
	}
}

func GetSiteHealth(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(400).SendString("Invalid site ID")
		}

		checks, err := models.GetHealthChecksBySiteID(db, id, 20)
		if err != nil {
			return c.Status(500).SendString("Failed to get health checks")
		}

		c.Set("Content-Type", "text/html")
		return partials.HealthChecks(checks).Render(c.Context(), c.Response().BodyWriter())
	}
}

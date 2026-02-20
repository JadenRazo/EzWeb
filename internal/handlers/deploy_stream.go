package handlers

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"time"

	"ezweb/internal/docker"
	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
)

func DeploySSE(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid site ID")
		}

		site, err := models.GetSiteByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Site not found")
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		_ = models.UpdateSiteStatus(db, id, "deploying")

		clientIP := c.IP()
		userAgent := c.Get("User-Agent")

		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			writeLine := func(msg string) {
				fmt.Fprintf(w, "data: %s\n\n", msg)
				_ = w.Flush()
			}

			writeLine("Starting deployment...")

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			var deployErr error
			if site.IsLocal && site.ComposePath != "" {
				writeLine("Running docker compose up for local site...")
				deployErr = docker.LocalComposeUp(ctx, site.ComposePath)
			} else {
				if !site.ServerID.Valid {
					writeLine("ERROR: No server assigned to this site")
					_ = models.UpdateSiteStatus(db, id, "error")
					writeLine("[DONE]")
					return
				}

				server, sErr := models.GetServerByID(db, int(site.ServerID.Int64))
				if sErr != nil {
					writeLine("ERROR: Assigned server not found")
					_ = models.UpdateSiteStatus(db, id, "error")
					writeLine("[DONE]")
					return
				}

				writeLine(fmt.Sprintf("Connecting to server %s...", server.Name))
				writeLine("Deploying containers...")
				envContent, _ := models.RenderEnvFile(db, id)
				deployErr = docker.DeploySite(
					server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey,
					site.Domain, site.TemplateSlug, site.ContainerName, site.Port, envContent,
				)
			}

			if deployErr != nil {
				log.Printf("SSE deploy failed for site %d (%s): %v", id, site.Domain, deployErr)
				writeLine(fmt.Sprintf("ERROR: %s", deployErr.Error()))
				_ = models.UpdateSiteStatus(db, id, "error")
			} else {
				writeLine("Deployment completed successfully!")
				_ = models.UpdateSiteStatus(db, id, "running")
				models.LogActivityWithContext(db, "site", id, "deployed", "Deployed site "+site.Domain, clientIP, userAgent)
			}

			writeLine("[DONE]")
		})

		return nil
	}
}

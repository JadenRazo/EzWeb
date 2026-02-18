package handlers

import (
	"database/sql"
	"log"
	"strconv"

	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

func ListServers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		servers, err := models.GetAllServers(db)
		if err != nil {
			log.Printf("failed to list servers: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load servers")
		}
		c.Set("Content-Type", "text/html")
		return pages.Servers(servers).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateServerHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		port, err := strconv.Atoi(c.FormValue("ssh_port", "22"))
		if err != nil {
			port = 22
		}

		s := &models.Server{
			Name:       c.FormValue("name"),
			Host:       c.FormValue("host"),
			SSHPort:    port,
			SSHUser:    c.FormValue("ssh_user", "root"),
			SSHKeyPath: c.FormValue("ssh_key_path"),
			Status:     "unknown",
		}

		if s.Name == "" || s.Host == "" || s.SSHKeyPath == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Name, host, and SSH key path are required")
		}

		if err := models.CreateServer(db, s); err != nil {
			log.Printf("failed to create server: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create server")
		}

		models.LogActivity(db, "server", s.ID, "created", "Added server "+s.Name)

		// Reload from DB to get full timestamps
		server, err := models.GetServerByID(db, s.ID)
		if err != nil {
			log.Printf("failed to fetch created server: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Server created but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.ServerRow(*server).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/servers")
	}
}

func EditServerForm(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid server ID")
		}

		server, err := models.GetServerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Server not found")
		}

		c.Set("Content-Type", "text/html")
		return partials.ServerEditRow(*server).Render(c.Context(), c.Response().BodyWriter())
	}
}

func UpdateServerHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid server ID")
		}

		port, err := strconv.Atoi(c.FormValue("ssh_port", "22"))
		if err != nil {
			port = 22
		}

		s := &models.Server{
			ID:         id,
			Name:       c.FormValue("name"),
			Host:       c.FormValue("host"),
			SSHPort:    port,
			SSHUser:    c.FormValue("ssh_user", "root"),
			SSHKeyPath: c.FormValue("ssh_key_path"),
		}

		if s.Name == "" || s.Host == "" || s.SSHKeyPath == "" {
			return c.Status(fiber.StatusBadRequest).SendString("Name, host, and SSH key path are required")
		}

		if err := models.UpdateServer(db, s); err != nil {
			log.Printf("failed to update server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update server")
		}

		models.LogActivity(db, "server", id, "updated", "Updated server "+s.Name)

		server, err := models.GetServerByID(db, id)
		if err != nil {
			log.Printf("failed to reload server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Server updated but failed to reload")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.ServerRow(*server).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/servers")
	}
}

func CancelEditServer(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid server ID")
		}

		server, err := models.GetServerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Server not found")
		}

		c.Set("Content-Type", "text/html")
		return partials.ServerRow(*server).Render(c.Context(), c.Response().BodyWriter())
	}
}

func DeleteServerHandler(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid server ID")
		}

		server, err := models.GetServerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Server not found")
		}

		if err := models.DeleteServer(db, id); err != nil {
			log.Printf("failed to delete server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete server")
		}

		models.LogActivity(db, "server", id, "deleted", "Deleted server "+server.Name)

		if c.Get("HX-Request") != "" {
			// Return empty body so HTMX removes the row via hx-swap="outerHTML"
			return c.SendString("")
		}
		return c.Redirect("/servers")
	}
}

func TestServerConnection(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("Invalid server ID")
		}

		server, err := models.GetServerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusNotFound).SendString("Server not found")
		}

		version, err := sshutil.TestConnection(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath)
		status := "online"
		if err != nil {
			log.Printf("connection test failed for server %d (%s): %v", id, server.Host, err)
			status = "offline"
		} else {
			log.Printf("server %d (%s) is online, Docker %s", id, server.Host, version)
		}

		if err := models.UpdateServerStatus(db, id, status); err != nil {
			log.Printf("failed to update status for server %d: %v", id, err)
		}

		// Reload to get updated status and timestamps
		server, err = models.GetServerByID(db, id)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to reload server")
		}

		if c.Get("HX-Request") != "" {
			c.Set("Content-Type", "text/html")
			return partials.ServerRow(*server).Render(c.Context(), c.Response().BodyWriter())
		}
		return c.Redirect("/servers")
	}
}

package handlers

import (
	"context"
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"ezweb/internal/docker"
	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"
	"ezweb/views/pages"
	"ezweb/views/partials"

	"github.com/gofiber/fiber/v2"
)

// validateSSHKeyPath checks that the SSH key path resolves to a file inside
// the allowed SSH key directory. If allowedDir is empty, only ~/.ssh is
// permitted. Returns an error string if invalid, empty string if valid.
func validateSSHKeyPath(keyPath string, allowedDir string) string {
	cleaned := filepath.Clean(keyPath)

	if allowedDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			allowedDir = "/root/.ssh"
		} else {
			allowedDir = filepath.Join(home, ".ssh")
		}
	}
	allowedDir = filepath.Clean(allowedDir)

	if !strings.HasPrefix(cleaned, allowedDir+string(filepath.Separator)) && cleaned != allowedDir {
		return "SSH key path must be inside " + allowedDir
	}
	return ""
}

func ListServers(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		servers, err := models.GetAllServers(db)
		if err != nil {
			log.Printf("failed to list servers: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to load servers")
		}
		localInfo := docker.GetLocalServerInfo(context.Background())
		c.Set("Content-Type", "text/html")
		return pages.Servers(localInfo, servers).Render(c.Context(), c.Response().BodyWriter())
	}
}

func CreateServerHandler(db *sql.DB, sshKeyDir ...string) fiber.Handler {
	allowedDir := ""
	if len(sshKeyDir) > 0 {
		allowedDir = sshKeyDir[0]
	}
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

		if msg := validateSSHKeyPath(s.SSHKeyPath, allowedDir); msg != "" {
			return c.Status(fiber.StatusBadRequest).SendString(msg)
		}

		if _, err := os.Stat(s.SSHKeyPath); err != nil {
			return c.Status(fiber.StatusBadRequest).SendString("SSH key file not found")
		}

		if err := models.CreateServer(db, s); err != nil {
			log.Printf("failed to create server: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to create server")
		}

		models.LogActivityWithContext(db, "server", s.ID, "created", "Added server "+s.Name, c.IP(), c.Get("User-Agent"))

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

func UpdateServerHandler(db *sql.DB, sshKeyDir ...string) fiber.Handler {
	allowedDir := ""
	if len(sshKeyDir) > 0 {
		allowedDir = sshKeyDir[0]
	}
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

		if msg := validateSSHKeyPath(s.SSHKeyPath, allowedDir); msg != "" {
			return c.Status(fiber.StatusBadRequest).SendString(msg)
		}

		if err := models.UpdateServer(db, s); err != nil {
			log.Printf("failed to update server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to update server")
		}

		models.LogActivityWithContext(db, "server", id, "updated", "Updated server "+s.Name, c.IP(), c.Get("User-Agent"))

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

		// Detach sites from this server so they retain their configuration but
		// are no longer associated with a now-deleted server.
		if _, err := db.Exec("UPDATE sites SET server_id = NULL WHERE server_id = ?", id); err != nil {
			log.Printf("failed to detach sites from server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to detach server sites")
		}

		if err := models.DeleteServer(db, id); err != nil {
			log.Printf("failed to delete server %d: %v", id, err)
			return c.Status(fiber.StatusInternalServerError).SendString("Failed to delete server")
		}

		models.LogActivityWithContext(db, "server", id, "deleted", "Deleted server "+server.Name, c.IP(), c.Get("User-Agent"))

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

		// Fetch and pin host key on first connection, then authenticate with the pinned key.
		if server.SSHHostKey == "" {
			hostKey, hkErr := sshutil.GetHostKey(server.Host, server.SSHPort)
			if hkErr != nil {
				log.Printf("failed to probe host key for server %d (%s): %v", id, server.Host, hkErr)
				if err := models.UpdateServerStatus(db, id, "offline"); err != nil {
					log.Printf("failed to update status for server %d: %v", id, err)
				}
				return c.Status(fiber.StatusBadRequest).SendString("Failed to retrieve host key: " + hkErr.Error())
			}
			if dbErr := models.UpdateServerHostKey(db, id, hostKey); dbErr != nil {
				log.Printf("failed to store host key for server %d: %v", id, dbErr)
			}
			server.SSHHostKey = hostKey
		}

		version, err := sshutil.TestConnection(server.Host, server.SSHPort, server.SSHUser, server.SSHKeyPath, server.SSHHostKey)
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

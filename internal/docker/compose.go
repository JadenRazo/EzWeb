package docker

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"text/template"

	"ezweb/internal/templates"

	sshutil "ezweb/internal/ssh"

	"github.com/pkg/sftp"
)

// ComposeVars holds the variables injected into Docker Compose templates.
type ComposeVars struct {
	ContainerName  string
	Port           int
	Domain         string
	DBPassword     string
	DBRootPassword string
}

// RenderCompose loads the embedded compose template for the given slug
// and renders it with the provided variables.
func RenderCompose(templateSlug string, vars ComposeVars) (string, error) {
	tmplContent, err := templates.GetComposeTemplate(templateSlug)
	if err != nil {
		return "", fmt.Errorf("failed to load compose template %q: %w", templateSlug, err)
	}

	tmpl, err := template.New(templateSlug).Parse(tmplContent)
	if err != nil {
		return "", fmt.Errorf("failed to parse compose template %q: %w", templateSlug, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("failed to render compose template %q: %w", templateSlug, err)
	}

	return buf.String(), nil
}

// generatePassword creates a random hex string of the specified length.
func generatePassword(length int) string {
	b := make([]byte, length)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:length]
}

// DeploySite renders a compose template, uploads it to the remote server via
// SFTP, and runs docker compose up to start the site containers.
func DeploySite(host string, port int, user string, keyPath string, domain string, templateSlug string, containerName string, sitePort int) error {
	vars := ComposeVars{
		ContainerName:  containerName,
		Port:           sitePort,
		Domain:         domain,
		DBPassword:     generatePassword(16),
		DBRootPassword: generatePassword(20),
	}

	rendered, err := RenderCompose(templateSlug, vars)
	if err != nil {
		return fmt.Errorf("failed to render compose for %s: %w", containerName, err)
	}

	sshClient, err := sshutil.NewClient(host, port, user, keyPath)
	if err != nil {
		return fmt.Errorf("SSH connect failed for %s:%d: %w", host, port, err)
	}
	defer sshClient.Close()

	remotePath := fmt.Sprintf("/opt/ezweb/%s", containerName)
	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("mkdir -p %s", remotePath)); err != nil {
		return fmt.Errorf("failed to create remote directory %s: %w", remotePath, err)
	}

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("failed to create SFTP session: %w", err)
	}
	defer sftpClient.Close()

	composeFile := fmt.Sprintf("%s/docker-compose.yml", remotePath)
	f, err := sftpClient.Create(composeFile)
	if err != nil {
		return fmt.Errorf("failed to create remote file %s: %w", composeFile, err)
	}
	if _, err := f.Write([]byte(rendered)); err != nil {
		f.Close()
		return fmt.Errorf("failed to write compose file: %w", err)
	}
	f.Close()

	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("cd %s && docker compose up -d", remotePath)); err != nil {
		return fmt.Errorf("docker compose up failed for %s: %w", containerName, err)
	}

	return nil
}

// StopSiteRemote stops the site containers on a remote server.
func StopSiteRemote(host string, port int, user string, keyPath string, containerName string) error {
	sshClient, err := sshutil.NewClient(host, port, user, keyPath)
	if err != nil {
		return fmt.Errorf("SSH connect failed: %w", err)
	}
	defer sshClient.Close()

	remotePath := fmt.Sprintf("/opt/ezweb/%s", containerName)
	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("cd %s && docker compose stop", remotePath)); err != nil {
		return fmt.Errorf("docker compose stop failed for %s: %w", containerName, err)
	}
	return nil
}

// StartSiteRemote starts the site containers on a remote server.
func StartSiteRemote(host string, port int, user string, keyPath string, containerName string) error {
	sshClient, err := sshutil.NewClient(host, port, user, keyPath)
	if err != nil {
		return fmt.Errorf("SSH connect failed: %w", err)
	}
	defer sshClient.Close()

	remotePath := fmt.Sprintf("/opt/ezweb/%s", containerName)
	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("cd %s && docker compose start", remotePath)); err != nil {
		return fmt.Errorf("docker compose start failed for %s: %w", containerName, err)
	}
	return nil
}

// RestartSiteRemote restarts the site containers on a remote server.
func RestartSiteRemote(host string, port int, user string, keyPath string, containerName string) error {
	sshClient, err := sshutil.NewClient(host, port, user, keyPath)
	if err != nil {
		return fmt.Errorf("SSH connect failed: %w", err)
	}
	defer sshClient.Close()

	remotePath := fmt.Sprintf("/opt/ezweb/%s", containerName)
	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("cd %s && docker compose restart", remotePath)); err != nil {
		return fmt.Errorf("docker compose restart failed for %s: %w", containerName, err)
	}
	return nil
}

// RemoveSiteRemote tears down the site containers and removes volumes on a remote server.
func RemoveSiteRemote(host string, port int, user string, keyPath string, containerName string) error {
	sshClient, err := sshutil.NewClient(host, port, user, keyPath)
	if err != nil {
		return fmt.Errorf("SSH connect failed: %w", err)
	}
	defer sshClient.Close()

	remotePath := fmt.Sprintf("/opt/ezweb/%s", containerName)
	if _, err := sshutil.RunCommand(sshClient, fmt.Sprintf("cd %s && docker compose down -v", remotePath)); err != nil {
		return fmt.Errorf("docker compose down failed for %s: %w", containerName, err)
	}
	return nil
}

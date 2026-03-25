package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

type ComposeProject struct {
	Name       string `json:"Name"`
	Status     string `json:"Status"`
	ConfigFile string `json:"ConfigFiles"`
}

type ScannedProject struct {
	Name       string
	Path       string
	Status     string
	ConfigFile string
}

func ScanLocalProjects(ctx context.Context) ([]ScannedProject, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", "compose", "ls", "--format", "json", "--all")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker compose ls failed: %w\n%s", err, string(out))
	}

	var projects []ComposeProject
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil, fmt.Errorf("failed to parse docker compose ls output: %w", err)
	}

	var result []ScannedProject
	for _, p := range projects {
		path := extractPath(p.ConfigFile)
		result = append(result, ScannedProject{
			Name:       p.Name,
			Path:       path,
			Status:     p.Status,
			ConfigFile: p.ConfigFile,
		})
	}

	return result, nil
}

func extractPath(configFiles string) string {
	// ConfigFiles is typically the full path to docker-compose.yml
	// We want the directory containing it
	if configFiles == "" {
		return ""
	}
	// Take the first config file if there are multiple (comma separated)
	first := configFiles
	for i, c := range configFiles {
		if c == ',' {
			first = configFiles[:i]
			break
		}
	}
	// Strip the filename to get the directory
	lastSlash := -1
	for i, c := range first {
		if c == '/' {
			lastSlash = i
		}
	}
	if lastSlash > 0 {
		return first[:lastSlash]
	}
	return first
}

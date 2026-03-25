package docker

import (
	"encoding/json"
	"strings"

	sshutil "ezweb/internal/ssh"

	"golang.org/x/crypto/ssh"
)

// RemoteServerStats holds resource usage information gathered from a remote server.
type RemoteServerStats struct {
	CPUUsage      string // e.g. "23%" or raw load average
	MemoryTotal   string // e.g. "8G"
	MemoryUsed    string // e.g. "3.2G"
	MemoryPercent string // e.g. "40%"
	DiskTotal     string // e.g. "100G"
	DiskUsed      string // e.g. "45G"
	DiskPercent   string // e.g. "45%"
	Uptime        string // e.g. "up 14 days, 3:22"
	LoadAverage   string // e.g. "0.15, 0.10, 0.09"
}

// RemoteContainer represents a single container reported by `docker ps -a` on a remote host.
type RemoteContainer struct {
	ID      string
	Name    string
	Image   string
	Status  string
	State   string // running, exited, etc.
	Ports   string
	Created string
}

// remoteContainerJSON mirrors the JSON shape emitted by the docker ps --format template.
type remoteContainerJSON struct {
	ID      string `json:"ID"`
	Name    string `json:"Name"`
	Image   string `json:"Image"`
	Status  string `json:"Status"`
	State   string `json:"State"`
	Ports   string `json:"Ports"`
	Created string `json:"Created"`
}

// ScanRemoteProjects runs `docker compose ls` on the remote host and returns a
// slice of ScannedProject values. If Docker Compose is not available on the
// remote host the function returns an empty slice rather than an error.
func ScanRemoteProjects(client *ssh.Client) ([]ScannedProject, error) {
	out, err := sshutil.RunCommand(client, "docker compose ls --format json --all")
	if err != nil {
		// Treat a missing docker/compose binary as a non-fatal condition so
		// callers can still display partial server information.
		if isDockerMissing(out) {
			return []ScannedProject{}, nil
		}
		return nil, err
	}

	if strings.TrimSpace(out) == "" || strings.TrimSpace(out) == "[]" {
		return []ScannedProject{}, nil
	}

	var projects []ComposeProject
	if err := json.Unmarshal([]byte(out), &projects); err != nil {
		// Output may contain a warning line before the JSON on some hosts.
		// Attempt to find and extract the JSON array.
		if cleaned := extractJSONArray(out); cleaned != "" {
			if err2 := json.Unmarshal([]byte(cleaned), &projects); err2 != nil {
				return nil, err2
			}
		} else {
			return nil, err
		}
	}

	result := make([]ScannedProject, 0, len(projects))
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

// GetRemoteContainers runs `docker ps -a` on the remote host and returns all
// containers regardless of their state.
func GetRemoteContainers(client *ssh.Client) ([]RemoteContainer, error) {
	// Each line is a self-contained JSON object produced by the Go template.
	const format = `{"ID":"{{.ID}}","Name":"{{.Names}}","Image":"{{.Image}}","Status":"{{.Status}}","State":"{{.State}}","Ports":"{{.Ports}}","Created":"{{.CreatedAt}}"}`
	cmd := `docker ps -a --format '` + format + `'`

	out, err := sshutil.RunCommand(client, cmd)
	if err != nil {
		if isDockerMissing(out) {
			return []RemoteContainer{}, nil
		}
		return nil, err
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	result := make([]RemoteContainer, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var c remoteContainerJSON
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			// Skip malformed lines rather than aborting the whole result.
			continue
		}
		result = append(result, RemoteContainer{
			ID:      c.ID,
			Name:    c.Name,
			Image:   c.Image,
			Status:  c.Status,
			State:   c.State,
			Ports:   c.Ports,
			Created: c.Created,
		})
	}
	return result, nil
}

// GetRemoteServerStats collects CPU, memory, disk, uptime, and load average
// information from the remote host. Individual fields are set to "N/A" when
// the underlying command fails so that a partial failure does not prevent the
// caller from displaying the values that were successfully retrieved.
func GetRemoteServerStats(client *ssh.Client) (RemoteServerStats, error) {
	stats := RemoteServerStats{
		CPUUsage:      "N/A",
		MemoryTotal:   "N/A",
		MemoryUsed:    "N/A",
		MemoryPercent: "N/A",
		DiskTotal:     "N/A",
		DiskUsed:      "N/A",
		DiskPercent:   "N/A",
		Uptime:        "N/A",
		LoadAverage:   "N/A",
	}

	// --- uptime + load average ---
	if out, err := sshutil.RunCommand(client, "uptime"); err == nil {
		stats.Uptime, stats.LoadAverage, stats.CPUUsage = parseUptime(out)
	}

	// --- memory ---
	if out, err := sshutil.RunCommand(client, "free -h | awk '/^Mem:/{print $2, $3}'"); err == nil {
		total, used := parseMemory(out)
		stats.MemoryTotal = total
		stats.MemoryUsed = used
	}
	// Get memory percentage using raw bytes for accuracy.
	if out, err := sshutil.RunCommand(client, `free | awk '/^Mem:/{printf "%.0f%%", $3/$2*100}'`); err == nil && out != "" {
		stats.MemoryPercent = out
	}

	// --- disk (root filesystem) ---
	if out, err := sshutil.RunCommand(client, "df -h / | awk 'NR==2{print $2, $3, $5}'"); err == nil {
		total, used, pct := parseDisk(out)
		stats.DiskTotal = total
		stats.DiskUsed = used
		stats.DiskPercent = pct
	}

	return stats, nil
}

// ---------------------------------------------------------------------------
// parsing helpers
// ---------------------------------------------------------------------------

// parseUptime extracts the uptime string, load average, and a derived CPU
// usage indicator from the output of the `uptime` command.
//
// Example output formats:
//
//	14:22:05 up 14 days,  3:22,  1 user,  load average: 0.15, 0.10, 0.09
//	 14:22:05 up  2:11,  0 users,  load average: 0.01, 0.04, 0.00
func parseUptime(out string) (uptime, loadAvg, cpuUsage string) {
	uptime = "N/A"
	loadAvg = "N/A"
	cpuUsage = "N/A"

	out = strings.TrimSpace(out)
	if out == "" {
		return
	}

	// Extract load average section.
	if idx := strings.Index(out, "load average:"); idx != -1 {
		loadAvg = strings.TrimSpace(out[idx+len("load average:"):])
		// Use the 1-minute load as the CPU usage indicator.
		parts := strings.Split(loadAvg, ",")
		if len(parts) > 0 {
			cpuUsage = strings.TrimSpace(parts[0]) + " (load)"
		}
	} else if idx := strings.Index(out, "load averages:"); idx != -1 {
		// BSD variant uses "load averages:"
		loadAvg = strings.TrimSpace(out[idx+len("load averages:"):])
		parts := strings.Split(loadAvg, " ")
		if len(parts) > 0 {
			cpuUsage = strings.TrimSpace(parts[0]) + " (load)"
		}
	}

	// Extract the "up ..." portion.
	if idx := strings.Index(out, " up "); idx != -1 {
		rest := out[idx+4:] // skip " up "
		// The uptime ends at the first comma that introduces user count or load.
		// We conservatively grab everything before "load average" and strip
		// trailing user-count fields.
		if laIdx := strings.Index(rest, "load average"); laIdx != -1 {
			rest = rest[:laIdx]
		}
		// Strip trailing comma, whitespace, and user-count fragment.
		rest = strings.TrimRight(rest, ", ")
		// Remove " X user(s)" suffix if present.
		if uIdx := strings.LastIndex(rest, ","); uIdx != -1 {
			candidate := strings.TrimSpace(rest[uIdx+1:])
			if strings.Contains(candidate, "user") {
				rest = strings.TrimRight(rest[:uIdx], ", ")
			}
		}
		uptime = "up " + strings.TrimSpace(rest)
	}

	return
}

// parseMemory splits the two-field output of `free -h | awk '/^Mem:/{print $2, $3}'`.
func parseMemory(out string) (total, used string) {
	out = strings.TrimSpace(out)
	if out == "" {
		return "N/A", "N/A"
	}
	parts := strings.Fields(out)
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return out, "N/A"
}

// parseDisk splits the three-field output of the df awk expression.
func parseDisk(out string) (total, used, percent string) {
	out = strings.TrimSpace(out)
	if out == "" {
		return "N/A", "N/A", "N/A"
	}
	parts := strings.Fields(out)
	if len(parts) >= 3 {
		return parts[0], parts[1], parts[2]
	}
	if len(parts) == 2 {
		return parts[0], parts[1], "N/A"
	}
	return out, "N/A", "N/A"
}

// isDockerMissing returns true when the command output suggests Docker or
// Docker Compose is not installed on the remote host.
func isDockerMissing(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "command not found") ||
		strings.Contains(lower, "no such file") ||
		strings.Contains(lower, "not found")
}

// extractJSONArray attempts to locate and return the first JSON array found
// within s. This handles cases where a remote host emits warning lines before
// the actual JSON output.
func extractJSONArray(s string) string {
	start := strings.Index(s, "[")
	if start == -1 {
		return ""
	}
	end := strings.LastIndex(s, "]")
	if end == -1 || end < start {
		return ""
	}
	return s[start : end+1]
}

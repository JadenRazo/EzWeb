package mcptools

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"

	"ezweb/internal/docker"
	"ezweb/internal/models"
	sshutil "ezweb/internal/ssh"

	"github.com/mark3labs/mcp-go/mcp"
)

type handlers struct {
	db *sql.DB
}

func (h *handlers) resolveSite(args map[string]any) (*models.Site, error) {
	if id, ok := args["site_id"]; ok {
		siteID, err := toInt(id)
		if err != nil {
			return nil, fmt.Errorf("invalid site_id: %w", err)
		}
		return models.GetSiteByID(h.db, siteID)
	}
	if domain, ok := args["domain"]; ok {
		d, _ := domain.(string)
		if d == "" {
			return nil, fmt.Errorf("domain cannot be empty")
		}
		return models.GetSiteByDomain(h.db, d)
	}
	return nil, fmt.Errorf("provide either site_id or domain")
}

func (h *handlers) listSites(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	sites, err := models.GetAllSites(h.db)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list sites: %v", err)), nil
	}

	// Apply filters
	statusFilter, _ := args["status"].(string)
	var serverIDFilter int
	if sid, ok := args["server_id"]; ok {
		serverIDFilter, _ = toInt(sid)
	}

	var filtered []SiteDTO
	for _, s := range sites {
		if statusFilter != "" && s.Status != statusFilter {
			continue
		}
		if serverIDFilter > 0 && (!s.ServerID.Valid || int(s.ServerID.Int64) != serverIDFilter) {
			continue
		}
		filtered = append(filtered, SiteToDTO(s))
	}

	return jsonResult(filtered)
}

func (h *handlers) getSiteLogs(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	site, err := h.resolveSite(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	tail := 100
	if t, ok := args["tail"]; ok {
		if v, err := toInt(t); err == nil && v > 0 {
			tail = v
		}
	}

	var logs string
	if site.IsLocal && site.ComposePath != "" {
		logs, err = docker.LocalComposeLogs(context.Background(), site.ComposePath, tail)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get local logs: %v", err)), nil
		}
	} else if site.ServerID.Valid {
		srv, err := models.GetServerByID(h.db, int(site.ServerID.Int64))
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("server not found: %v", err)), nil
		}
		client, err := sshutil.NewClient(srv.Host, srv.SSHPort, srv.SSHUser, srv.SSHKeyPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("SSH connection failed: %v", err)), nil
		}
		defer client.Close()

		cmd := fmt.Sprintf("cd /opt/ezweb/%s && docker compose logs --tail %d --no-color", site.ContainerName, tail)
		logs, err = sshutil.RunCommand(client, cmd)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get remote logs: %v", err)), nil
		}
	} else {
		return mcp.NewToolResultError("site has no compose path (local) or server assignment (remote)"), nil
	}

	result := map[string]any{
		"site_id": site.ID,
		"domain":  site.Domain,
		"tail":    tail,
		"logs":    logs,
	}
	return jsonResult(result)
}

func (h *handlers) getSiteHealth(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	site, err := h.resolveSite(args)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	limit := 10
	if l, ok := args["limit"]; ok {
		if v, err := toInt(l); err == nil && v > 0 {
			limit = v
		}
	}

	checks, err := models.GetHealthChecksBySiteID(h.db, site.ID, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get health checks: %v", err)), nil
	}

	var dtos []HealthCheckDTO
	for _, c := range checks {
		dtos = append(dtos, HealthCheckToDTO(c))
	}

	result := map[string]any{
		"site_id": site.ID,
		"domain":  site.Domain,
		"status":  site.Status,
		"checks":  dtos,
	}
	return jsonResult(result)
}

func (h *handlers) getSiteErrors(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	hours := 24
	if hr, ok := args["hours"]; ok {
		if v, err := toInt(hr); err == nil && v > 0 {
			hours = v
		}
	}

	// Sites with error/stopped status
	sites, err := models.GetAllSites(h.db)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to query sites: %v", err)), nil
	}

	type problemSite struct {
		Site          SiteDTO        `json:"site"`
		Problems      []string       `json:"problems"`
		LatestCheck   *HealthCheckDTO `json:"latest_check,omitempty"`
	}

	// Find sites with failed health checks in the time window
	query := `SELECT DISTINCT site_id FROM health_checks
		WHERE (http_status < 200 OR http_status >= 400 OR container_status != 'running')
		AND checked_at >= datetime('now', ?)
		ORDER BY checked_at DESC`

	rows, err := h.db.Query(query, fmt.Sprintf("-%d hours", hours))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to query health checks: %v", err)), nil
	}
	defer rows.Close()

	failedHealthSiteIDs := make(map[int]bool)
	for rows.Next() {
		var siteID int
		if err := rows.Scan(&siteID); err == nil {
			failedHealthSiteIDs[siteID] = true
		}
	}

	var problems []problemSite
	for _, s := range sites {
		var probs []string
		if s.Status == "error" {
			probs = append(probs, "site status is error")
		}
		if s.Status == "stopped" {
			probs = append(probs, "site is stopped")
		}
		if failedHealthSiteIDs[s.ID] {
			probs = append(probs, "failed health checks in the last "+strconv.Itoa(hours)+" hours")
		}
		if len(probs) == 0 {
			continue
		}

		ps := problemSite{
			Site:     SiteToDTO(s),
			Problems: probs,
		}

		latest, err := models.GetLatestHealthCheck(h.db, s.ID)
		if err == nil {
			dto := HealthCheckToDTO(*latest)
			ps.LatestCheck = &dto
		}

		problems = append(problems, ps)
	}

	result := map[string]any{
		"hours":         hours,
		"problem_count": len(problems),
		"problems":      problems,
	}
	return jsonResult(result)
}

func (h *handlers) getActivityLog(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	limit := 20
	if l, ok := args["limit"]; ok {
		if v, err := toInt(l); err == nil && v > 0 {
			limit = v
		}
	}

	activities, err := models.GetRecentActivities(h.db, limit)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to get activities: %v", err)), nil
	}

	entityFilter, _ := args["entity_type"].(string)

	var dtos []ActivityDTO
	for _, a := range activities {
		if entityFilter != "" && a.EntityType != entityFilter {
			continue
		}
		dtos = append(dtos, ActivityToDTO(a))
	}

	return jsonResult(dtos)
}

func (h *handlers) listServers(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	servers, err := models.GetAllServers(h.db)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to list servers: %v", err)), nil
	}

	var dtos []ServerDTO
	for _, s := range servers {
		dtos = append(dtos, ServerToDTO(s))
	}

	return jsonResult(dtos)
}

func (h *handlers) getServerStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()

	sid, ok := args["server_id"]
	if !ok {
		return mcp.NewToolResultError("server_id is required"), nil
	}

	serverID, err := toInt(sid)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("invalid server_id: %v", err)), nil
	}

	srv, err := models.GetServerByID(h.db, serverID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("server not found: %v", err)), nil
	}

	dockerVersion, testErr := sshutil.TestConnection(srv.Host, srv.SSHPort, srv.SSHUser, srv.SSHKeyPath)

	status := "online"
	if testErr != nil {
		status = "offline"
	}

	// Update server status in DB
	_ = models.UpdateServerStatus(h.db, srv.ID, status)

	result := map[string]any{
		"server_id":      srv.ID,
		"name":           srv.Name,
		"host":           srv.Host,
		"status":         status,
		"docker_version": dockerVersion,
	}
	if testErr != nil {
		result["error"] = testErr.Error()
	}

	return jsonResult(result)
}

// helpers

func toInt(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		return strconv.Atoi(val)
	case json.Number:
		n, err := val.Int64()
		return int(n), err
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

func jsonResult(data any) (*mcp.CallToolResult, error) {
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("failed to serialize result: %v", err)), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}

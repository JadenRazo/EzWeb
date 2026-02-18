package mcptools

import (
	"ezweb/internal/models"
	"time"
)

type SiteDTO struct {
	ID            int    `json:"id"`
	Domain        string `json:"domain"`
	ServerID      *int64 `json:"server_id,omitempty"`
	ServerName    string `json:"server_name,omitempty"`
	TemplateSlug  string `json:"template_slug,omitempty"`
	CustomerID    *int64 `json:"customer_id,omitempty"`
	CustomerName  string `json:"customer_name,omitempty"`
	ContainerName string `json:"container_name,omitempty"`
	Port          int    `json:"port"`
	Status        string `json:"status"`
	SSLEnabled    bool   `json:"ssl_enabled"`
	IsLocal       bool   `json:"is_local"`
	ComposePath   string `json:"compose_path,omitempty"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type ServerDTO struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	SSHPort   int    `json:"ssh_port"`
	SSHUser   string `json:"ssh_user"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type HealthCheckDTO struct {
	ID              int    `json:"id"`
	SiteID          int    `json:"site_id"`
	HTTPStatus      int    `json:"http_status"`
	LatencyMs       int    `json:"latency_ms"`
	ContainerStatus string `json:"container_status"`
	CheckedAt       string `json:"checked_at"`
}

type ActivityDTO struct {
	ID         int    `json:"id"`
	EntityType string `json:"entity_type"`
	EntityID   int    `json:"entity_id"`
	Action     string `json:"action"`
	Details    string `json:"details,omitempty"`
	CreatedAt  string `json:"created_at"`
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func SiteToDTO(s models.Site) SiteDTO {
	dto := SiteDTO{
		ID:            s.ID,
		Domain:        s.Domain,
		TemplateSlug:  s.TemplateSlug,
		ServerName:    s.ServerName,
		CustomerName:  s.CustomerName,
		ContainerName: s.ContainerName,
		Port:          s.Port,
		Status:        s.Status,
		SSLEnabled:    s.SSLEnabled,
		IsLocal:       s.IsLocal,
		ComposePath:   s.ComposePath,
		CreatedAt:     formatTime(s.CreatedAt),
		UpdatedAt:     formatTime(s.UpdatedAt),
	}
	if s.ServerID.Valid {
		dto.ServerID = &s.ServerID.Int64
	}
	if s.CustomerID.Valid {
		dto.CustomerID = &s.CustomerID.Int64
	}
	return dto
}

func ServerToDTO(s models.Server) ServerDTO {
	return ServerDTO{
		ID:        s.ID,
		Name:      s.Name,
		Host:      s.Host,
		SSHPort:   s.SSHPort,
		SSHUser:   s.SSHUser,
		Status:    s.Status,
		CreatedAt: formatTime(s.CreatedAt),
		UpdatedAt: formatTime(s.UpdatedAt),
	}
}

func HealthCheckToDTO(h models.HealthCheck) HealthCheckDTO {
	return HealthCheckDTO{
		ID:              h.ID,
		SiteID:          h.SiteID,
		HTTPStatus:      h.HTTPStatus,
		LatencyMs:       h.LatencyMs,
		ContainerStatus: h.ContainerStatus,
		CheckedAt:       h.CheckedAt,
	}
}

func ActivityToDTO(a models.Activity) ActivityDTO {
	return ActivityDTO{
		ID:         a.ID,
		EntityType: a.EntityType,
		EntityID:   a.EntityID,
		Action:     a.Action,
		Details:    a.Details,
		CreatedAt:  a.CreatedAt,
	}
}

package models

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Site struct {
	ID            int
	Domain        string
	ServerID      sql.NullInt64
	TemplateSlug  string
	CustomerID    sql.NullInt64
	ContainerName string
	Port          int
	Status        string
	SSLEnabled    bool
	IsLocal       bool
	ComposePath   string
	RoutingConfig *RoutingConfig
	SSLExpiry     sql.NullTime
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// Joined fields (not stored in DB)
	ServerName   string
	CustomerName string
}

// routingConfigJSON returns the JSON string for DB storage, or empty string if nil.
func (s *Site) routingConfigJSON() string {
	if s.RoutingConfig == nil || len(s.RoutingConfig.Rules) == 0 {
		return ""
	}
	b, err := json.Marshal(s.RoutingConfig)
	if err != nil {
		return ""
	}
	return string(b)
}

// parseRoutingConfig parses a JSON string into RoutingConfig.
func parseRoutingConfig(raw string) *RoutingConfig {
	if raw == "" {
		return nil
	}
	var rc RoutingConfig
	if err := json.Unmarshal([]byte(raw), &rc); err != nil {
		return nil
	}
	if len(rc.Rules) == 0 {
		return nil
	}
	return &rc
}

const siteSelectColumns = `
	s.id, s.domain, s.server_id, COALESCE(s.template_slug,''), s.customer_id,
	COALESCE(s.container_name,''), COALESCE(s.port,0), COALESCE(s.status,'pending'),
	COALESCE(s.ssl_enabled,0), COALESCE(s.is_local,0), COALESCE(s.compose_path,''),
	COALESCE(s.routing_config,''), s.ssl_expiry, s.created_at, s.updated_at,
	COALESCE(srv.name,''), COALESCE(c.name,'')`

const siteFromJoins = `
	FROM sites s
	LEFT JOIN servers srv ON s.server_id = srv.id
	LEFT JOIN customers c ON s.customer_id = c.id`

func scanSite(scanner interface{ Scan(dest ...interface{}) error }) (*Site, error) {
	var s Site
	var sslInt, localInt int
	var routingRaw string
	if err := scanner.Scan(
		&s.ID, &s.Domain, &s.ServerID, &s.TemplateSlug, &s.CustomerID,
		&s.ContainerName, &s.Port, &s.Status,
		&sslInt, &localInt, &s.ComposePath,
		&routingRaw, &s.SSLExpiry, &s.CreatedAt, &s.UpdatedAt,
		&s.ServerName, &s.CustomerName,
	); err != nil {
		return nil, err
	}
	s.SSLEnabled = sslInt == 1
	s.IsLocal = localInt == 1
	s.RoutingConfig = parseRoutingConfig(routingRaw)
	return &s, nil
}

func GetAllSites(db *sql.DB) ([]Site, error) {
	rows, err := db.Query(`SELECT ` + siteSelectColumns + siteFromJoins + ` ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site row: %w", err)
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

func GetSiteByID(db *sql.DB, id int) (*Site, error) {
	row := db.QueryRow(`SELECT `+siteSelectColumns+siteFromJoins+` WHERE s.id = ?`, id)
	s, err := scanSite(row)
	if err != nil {
		return nil, fmt.Errorf("site not found: %w", err)
	}
	return s, nil
}

func GetSitesByServerID(db *sql.DB, serverID int) ([]Site, error) {
	rows, err := db.Query(
		`SELECT `+siteSelectColumns+siteFromJoins+` WHERE s.server_id = ? ORDER BY s.created_at DESC`,
		serverID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites by server: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site row: %w", err)
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

func GetSitesByCustomerID(db *sql.DB, customerID int) ([]Site, error) {
	rows, err := db.Query(
		`SELECT `+siteSelectColumns+siteFromJoins+` WHERE s.customer_id = ? ORDER BY s.created_at DESC`,
		customerID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query sites by customer: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan site row: %w", err)
		}
		sites = append(sites, *s)
	}
	return sites, rows.Err()
}

func CreateSite(db *sql.DB, s *Site) error {
	sslInt := 0
	if s.SSLEnabled {
		sslInt = 1
	}
	localInt := 0
	if s.IsLocal {
		localInt = 1
	}

	result, err := db.Exec(
		`INSERT INTO sites (domain, server_id, template_slug, customer_id, container_name, port, status, ssl_enabled, is_local, compose_path, routing_config)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Domain, s.ServerID, s.TemplateSlug, s.CustomerID,
		s.ContainerName, s.Port, s.Status, sslInt, localInt, s.ComposePath, s.routingConfigJSON(),
	)
	if err != nil {
		return fmt.Errorf("failed to create site: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	s.ID = int(id)
	return nil
}

func UpdateSite(db *sql.DB, s *Site) error {
	sslInt := 0
	if s.SSLEnabled {
		sslInt = 1
	}
	localInt := 0
	if s.IsLocal {
		localInt = 1
	}

	_, err := db.Exec(
		`UPDATE sites SET domain = ?, server_id = ?, template_slug = ?, customer_id = ?,
		 container_name = ?, port = ?, status = ?, ssl_enabled = ?, is_local = ?, compose_path = ?,
		 routing_config = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		s.Domain, s.ServerID, s.TemplateSlug, s.CustomerID,
		s.ContainerName, s.Port, s.Status, sslInt, localInt, s.ComposePath,
		s.routingConfigJSON(), s.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update site: %w", err)
	}
	return nil
}

func UpdateSiteStatus(db *sql.DB, id int, status string) error {
	_, err := db.Exec(
		"UPDATE sites SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update site status: %w", err)
	}
	return nil
}

func DeleteSite(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM sites WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete site: %w", err)
	}
	return nil
}

func CountSites(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sites").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count sites: %w", err)
	}
	return count, nil
}

func GetSiteByDomain(db *sql.DB, domain string) (*Site, error) {
	row := db.QueryRow(`SELECT `+siteSelectColumns+siteFromJoins+` WHERE s.domain = ?`, domain)
	s, err := scanSite(row)
	if err != nil {
		return nil, fmt.Errorf("site not found: %w", err)
	}
	return s, nil
}

func GetSiteByComposePath(db *sql.DB, composePath string) (*Site, error) {
	row := db.QueryRow(`SELECT `+siteSelectColumns+siteFromJoins+` WHERE s.compose_path = ?`, composePath)
	s, err := scanSite(row)
	if err != nil {
		return nil, fmt.Errorf("site not found by compose_path: %w", err)
	}
	return s, nil
}

// SearchSites returns a page of sites filtered by an optional domain substring
// search and an optional exact status match. Either filter may be an empty
// string to indicate "no filter". It also returns the total count of matching
// rows so the caller can compute pagination metadata.
func SearchSites(db *sql.DB, query, status string, page, perPage int) ([]Site, int, error) {
	var conditions []string
	var args []interface{}

	if query != "" {
		conditions = append(conditions, "s.domain LIKE ? ESCAPE '\\'")
		escaped := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(query)
		args = append(args, "%"+escaped+"%")
	}
	if status != "" {
		conditions = append(conditions, "s.status = ?")
		args = append(args, status)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = " WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total matching rows for pagination metadata.
	var total int
	countArgs := make([]interface{}, len(args))
	copy(countArgs, args)
	countQuery := "SELECT COUNT(*) FROM sites s" + whereClause
	if err := db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count filtered sites: %w", err)
	}

	// Fetch the requested page.
	offset := (page - 1) * perPage
	listArgs := append(args, perPage, offset)
	listQuery := `SELECT ` + siteSelectColumns + siteFromJoins + whereClause +
		` ORDER BY s.created_at DESC LIMIT ? OFFSET ?`

	rows, err := db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query filtered sites: %w", err)
	}
	defer rows.Close()

	var sites []Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to scan site row: %w", err)
		}
		sites = append(sites, *s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("site row iteration error: %w", err)
	}
	return sites, total, nil
}

// UpdateSiteSSLExpiry stores the latest observed certificate expiry time for
// a site. It is called by the health checker after a successful TLS handshake.
func UpdateSiteSSLExpiry(db *sql.DB, siteID int, expiry time.Time) error {
	_, err := db.Exec(
		"UPDATE sites SET ssl_expiry = ? WHERE id = ?",
		expiry, siteID,
	)
	if err != nil {
		return fmt.Errorf("failed to update ssl_expiry for site %d: %w", siteID, err)
	}
	return nil
}

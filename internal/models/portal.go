package models

import (
	"database/sql"
	"fmt"
	"time"
)

// QuoteRequest is an inbound inquiry submitted through the public client portal.
type QuoteRequest struct {
	ID          int
	Name        string
	Email       string
	Phone       string
	Company     string
	ProjectType string
	Description string
	BudgetRange string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ClientToken holds a hashed magic-link token for passwordless client authentication.
type ClientToken struct {
	ID         int
	CustomerID int
	TokenHash  string
	ExpiresAt  time.Time
	CreatedAt  time.Time
}

// PortfolioItem is a showcase entry displayed on the public portfolio page.
type PortfolioItem struct {
	ID             int
	Title          string
	Description    string
	URL            string
	ScreenshotPath string
	TemplateSlug   string
	DisplayOrder   int
	IsVisible      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// --- QuoteRequest ---

func CreateQuoteRequest(db *sql.DB, q *QuoteRequest) error {
	result, err := db.Exec(
		`INSERT INTO quote_requests (name, email, phone, company, project_type, description, budget_range, status)
		 VALUES (?, ?, ?, ?, ?, ?, ?, 'new')`,
		q.Name, q.Email, q.Phone, q.Company, q.ProjectType, q.Description, q.BudgetRange,
	)
	if err != nil {
		return fmt.Errorf("failed to create quote request: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	q.ID = int(id)
	return nil
}

func GetQuoteRequestByID(db *sql.DB, id int) (*QuoteRequest, error) {
	q := &QuoteRequest{}
	err := db.QueryRow(
		`SELECT id, name, email, COALESCE(phone,''), COALESCE(company,''),
		        COALESCE(project_type,''), COALESCE(description,''), COALESCE(budget_range,''),
		        status, created_at, updated_at
		 FROM quote_requests WHERE id = ?`,
		id,
	).Scan(
		&q.ID, &q.Name, &q.Email, &q.Phone, &q.Company,
		&q.ProjectType, &q.Description, &q.BudgetRange,
		&q.Status, &q.CreatedAt, &q.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("quote request not found: %w", err)
	}
	return q, nil
}

func GetQuoteRequestsPaginated(db *sql.DB, limit, offset int) ([]QuoteRequest, error) {
	rows, err := db.Query(
		`SELECT id, name, email, COALESCE(phone,''), COALESCE(company,''),
		        COALESCE(project_type,''), COALESCE(description,''), COALESCE(budget_range,''),
		        status, created_at, updated_at
		 FROM quote_requests ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query quote requests: %w", err)
	}
	defer rows.Close()

	var results []QuoteRequest
	for rows.Next() {
		var q QuoteRequest
		if err := rows.Scan(
			&q.ID, &q.Name, &q.Email, &q.Phone, &q.Company,
			&q.ProjectType, &q.Description, &q.BudgetRange,
			&q.Status, &q.CreatedAt, &q.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote request: %w", err)
		}
		results = append(results, q)
	}
	return results, rows.Err()
}

func GetQuoteRequestsByStatusPaginated(db *sql.DB, status string, limit, offset int) ([]QuoteRequest, error) {
	rows, err := db.Query(
		`SELECT id, name, email, COALESCE(phone,''), COALESCE(company,''),
		        COALESCE(project_type,''), COALESCE(description,''), COALESCE(budget_range,''),
		        status, created_at, updated_at
		 FROM quote_requests WHERE status = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		status, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query quote requests by status: %w", err)
	}
	defer rows.Close()

	var results []QuoteRequest
	for rows.Next() {
		var q QuoteRequest
		if err := rows.Scan(
			&q.ID, &q.Name, &q.Email, &q.Phone, &q.Company,
			&q.ProjectType, &q.Description, &q.BudgetRange,
			&q.Status, &q.CreatedAt, &q.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote request: %w", err)
		}
		results = append(results, q)
	}
	return results, rows.Err()
}

func CountQuoteRequests(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM quote_requests").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count quote requests: %w", err)
	}
	return count, nil
}

func CountQuoteRequestsByStatus(db *sql.DB, status string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM quote_requests WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count quote requests by status: %w", err)
	}
	return count, nil
}

func UpdateQuoteRequestStatus(db *sql.DB, id int, status string) error {
	_, err := db.Exec(
		"UPDATE quote_requests SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update quote request %d status: %w", id, err)
	}
	return nil
}

func DeleteQuoteRequest(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM quote_requests WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete quote request %d: %w", id, err)
	}
	return nil
}

// --- ClientToken ---

func CreateClientToken(db *sql.DB, customerID int, tokenHash string, expiresAt time.Time) (*ClientToken, error) {
	result, err := db.Exec(
		"INSERT INTO client_tokens (customer_id, token_hash, expires_at) VALUES (?, ?, ?)",
		customerID, tokenHash, expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client token: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get last insert id: %w", err)
	}
	return &ClientToken{
		ID:         int(id),
		CustomerID: customerID,
		TokenHash:  tokenHash,
		ExpiresAt:  expiresAt,
	}, nil
}

func GetClientTokenByHash(db *sql.DB, hash string) (*ClientToken, error) {
	t := &ClientToken{}
	err := db.QueryRow(
		"SELECT id, customer_id, token_hash, expires_at, created_at FROM client_tokens WHERE token_hash = ?",
		hash,
	).Scan(&t.ID, &t.CustomerID, &t.TokenHash, &t.ExpiresAt, &t.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("client token not found: %w", err)
	}
	return t, nil
}

func DeleteClientToken(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM client_tokens WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete client token: %w", err)
	}
	return nil
}

func DeleteExpiredClientTokens(db *sql.DB) error {
	_, err := db.Exec("DELETE FROM client_tokens WHERE expires_at < datetime('now')")
	if err != nil {
		return fmt.Errorf("failed to delete expired client tokens: %w", err)
	}
	return nil
}

// --- PortfolioItem ---

func GetVisiblePortfolioItems(db *sql.DB) ([]PortfolioItem, error) {
	rows, err := db.Query(
		`SELECT id, title, COALESCE(description,''), COALESCE(url,''), COALESCE(screenshot_path,''),
		        COALESCE(template_slug,''), COALESCE(display_order,0), COALESCE(is_visible,1),
		        created_at, updated_at
		 FROM portfolio_items WHERE is_visible = 1 ORDER BY display_order ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query portfolio items: %w", err)
	}
	defer rows.Close()
	return scanPortfolioItems(rows)
}

func GetAllPortfolioItems(db *sql.DB) ([]PortfolioItem, error) {
	rows, err := db.Query(
		`SELECT id, title, COALESCE(description,''), COALESCE(url,''), COALESCE(screenshot_path,''),
		        COALESCE(template_slug,''), COALESCE(display_order,0), COALESCE(is_visible,1),
		        created_at, updated_at
		 FROM portfolio_items ORDER BY display_order ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query portfolio items: %w", err)
	}
	defer rows.Close()
	return scanPortfolioItems(rows)
}

func scanPortfolioItems(rows *sql.Rows) ([]PortfolioItem, error) {
	var items []PortfolioItem
	for rows.Next() {
		var p PortfolioItem
		var isVisibleInt int
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Description, &p.URL, &p.ScreenshotPath,
			&p.TemplateSlug, &p.DisplayOrder, &isVisibleInt,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan portfolio item: %w", err)
		}
		p.IsVisible = isVisibleInt == 1
		items = append(items, p)
	}
	return items, rows.Err()
}

func CreatePortfolioItem(db *sql.DB, p *PortfolioItem) error {
	isVisibleInt := 1
	if !p.IsVisible {
		isVisibleInt = 0
	}
	result, err := db.Exec(
		`INSERT INTO portfolio_items (title, description, url, screenshot_path, template_slug, display_order, is_visible)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Title, p.Description, p.URL, p.ScreenshotPath, p.TemplateSlug, p.DisplayOrder, isVisibleInt,
	)
	if err != nil {
		return fmt.Errorf("failed to create portfolio item: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

func UpdatePortfolioItem(db *sql.DB, p *PortfolioItem) error {
	isVisibleInt := 0
	if p.IsVisible {
		isVisibleInt = 1
	}
	_, err := db.Exec(
		`UPDATE portfolio_items
		 SET title = ?, description = ?, url = ?, screenshot_path = ?, template_slug = ?,
		     display_order = ?, is_visible = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		p.Title, p.Description, p.URL, p.ScreenshotPath, p.TemplateSlug,
		p.DisplayOrder, isVisibleInt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update portfolio item %d: %w", p.ID, err)
	}
	return nil
}

func DeletePortfolioItem(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM portfolio_items WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete portfolio item %d: %w", id, err)
	}
	return nil
}

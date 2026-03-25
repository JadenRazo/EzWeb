package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type Quote struct {
	ID              int
	PublicID        string
	CustomerID      sql.NullInt64
	CustomerName    string
	CustomerEmail   string
	CustomerPhone   string
	CustomerCompany string
	TemplateSlug    string
	DomainName      string
	DomainPrice     float64
	DomainRegistrar string
	SetupFee        float64
	MonthlyPrice    float64
	YearlyPrice     float64
	BillingCycle    string
	DiscountPercent float64
	TaxRate         float64
	Subtotal        float64
	TaxAmount       float64
	Total           float64
	Status          string
	Notes           string
	ValidUntil      string
	SentAt          sql.NullTime
	AcceptedAt      sql.NullTime
	RejectedAt      sql.NullTime
	ConvertedSiteID sql.NullInt64
	CreatedAt       time.Time
	UpdatedAt       time.Time

	// Populated via JOIN or secondary query
	TemplateName string
	Addons       []QuoteAddon
}

type QuoteAddon struct {
	ID        int
	QuoteID   int
	AddonID   int
	Quantity  int
	Price     float64
	PriceType string

	// Populated via JOIN with addons table
	AddonName string
}

const quoteSelectColumns = `
	q.id, q.public_id, q.customer_id,
	COALESCE(q.customer_name,''), COALESCE(q.customer_email,''),
	COALESCE(q.customer_phone,''), COALESCE(q.customer_company,''),
	COALESCE(q.template_slug,''), COALESCE(q.domain_name,''), COALESCE(q.domain_price,0),
	COALESCE(q.domain_registrar,''), q.setup_fee, q.monthly_price, q.yearly_price,
	COALESCE(q.billing_cycle,'monthly'), COALESCE(q.discount_percent,0), COALESCE(q.tax_rate,0),
	q.subtotal, q.tax_amount, q.total, q.status,
	COALESCE(q.notes,''), COALESCE(q.valid_until,''),
	q.sent_at, q.accepted_at, q.rejected_at, q.converted_site_id,
	q.created_at, q.updated_at,
	COALESCE(pt.label,'')`

const quoteFromJoins = `
	FROM quotes q
	LEFT JOIN pricing_tiers pt ON q.template_slug = pt.template_slug`

func scanQuote(scanner interface{ Scan(dest ...any) error }) (*Quote, error) {
	var q Quote
	err := scanner.Scan(
		&q.ID, &q.PublicID, &q.CustomerID,
		&q.CustomerName, &q.CustomerEmail, &q.CustomerPhone, &q.CustomerCompany,
		&q.TemplateSlug, &q.DomainName, &q.DomainPrice,
		&q.DomainRegistrar, &q.SetupFee, &q.MonthlyPrice, &q.YearlyPrice,
		&q.BillingCycle, &q.DiscountPercent, &q.TaxRate,
		&q.Subtotal, &q.TaxAmount, &q.Total, &q.Status,
		&q.Notes, &q.ValidUntil,
		&q.SentAt, &q.AcceptedAt, &q.RejectedAt, &q.ConvertedSiteID,
		&q.CreatedAt, &q.UpdatedAt,
		&q.TemplateName,
	)
	if err != nil {
		return nil, err
	}
	return &q, nil
}

// CreateQuote inserts a new quote, generating a UUID for its public identifier.
// The ID field on q is set to the newly inserted row's primary key.
func CreateQuote(db *sql.DB, q *Quote) error {
	q.PublicID = uuid.New().String()
	result, err := db.Exec(
		`INSERT INTO quotes (
			public_id, customer_id, customer_name, customer_email, customer_phone, customer_company,
			template_slug, domain_name, domain_price, domain_registrar,
			setup_fee, monthly_price, yearly_price, billing_cycle,
			discount_percent, tax_rate, subtotal, tax_amount, total,
			status, notes, valid_until
		) VALUES (
			?, ?, ?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?,
			?, ?, ?, ?, ?,
			?, ?, ?
		)`,
		q.PublicID, q.CustomerID, q.CustomerName, q.CustomerEmail, q.CustomerPhone, q.CustomerCompany,
		q.TemplateSlug, q.DomainName, q.DomainPrice, q.DomainRegistrar,
		q.SetupFee, q.MonthlyPrice, q.YearlyPrice, q.BillingCycle,
		q.DiscountPercent, q.TaxRate, q.Subtotal, q.TaxAmount, q.Total,
		q.Status, q.Notes, q.ValidUntil,
	)
	if err != nil {
		return fmt.Errorf("failed to create quote: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	q.ID = int(id)
	return nil
}

// UpdateQuote writes all mutable fields back to the database and bumps updated_at.
func UpdateQuote(db *sql.DB, q *Quote) error {
	_, err := db.Exec(
		`UPDATE quotes SET
			customer_id = ?, customer_name = ?, customer_email = ?, customer_phone = ?,
			customer_company = ?, template_slug = ?, domain_name = ?, domain_price = ?,
			domain_registrar = ?, setup_fee = ?, monthly_price = ?, yearly_price = ?,
			billing_cycle = ?, discount_percent = ?, tax_rate = ?, subtotal = ?,
			tax_amount = ?, total = ?, status = ?, notes = ?, valid_until = ?,
			updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		q.CustomerID, q.CustomerName, q.CustomerEmail, q.CustomerPhone,
		q.CustomerCompany, q.TemplateSlug, q.DomainName, q.DomainPrice,
		q.DomainRegistrar, q.SetupFee, q.MonthlyPrice, q.YearlyPrice,
		q.BillingCycle, q.DiscountPercent, q.TaxRate, q.Subtotal,
		q.TaxAmount, q.Total, q.Status, q.Notes, q.ValidUntil,
		q.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update quote %d: %w", q.ID, err)
	}
	return nil
}

func loadQuoteAddons(db *sql.DB, q *Quote) error {
	addons, err := GetQuoteAddons(db, q.ID)
	if err != nil {
		return err
	}
	q.Addons = addons
	return nil
}

// GetQuoteByID returns a quote by primary key, including its add-ons and template label.
func GetQuoteByID(db *sql.DB, id int) (*Quote, error) {
	row := db.QueryRow("SELECT "+quoteSelectColumns+quoteFromJoins+" WHERE q.id = ?", id)
	q, err := scanQuote(row)
	if err != nil {
		return nil, fmt.Errorf("quote not found: %w", err)
	}
	if err := loadQuoteAddons(db, q); err != nil {
		return nil, err
	}
	return q, nil
}

// GetQuoteByPublicID returns a quote by its UUID public identifier, including add-ons.
func GetQuoteByPublicID(db *sql.DB, publicID string) (*Quote, error) {
	row := db.QueryRow("SELECT "+quoteSelectColumns+quoteFromJoins+" WHERE q.public_id = ?", publicID)
	q, err := scanQuote(row)
	if err != nil {
		return nil, fmt.Errorf("quote not found: %w", err)
	}
	if err := loadQuoteAddons(db, q); err != nil {
		return nil, err
	}
	return q, nil
}

// DeleteQuote removes a quote by primary key. Cascading deletes on quote_addons
// are handled by the ON DELETE CASCADE foreign key in the schema.
func DeleteQuote(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM quotes WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete quote %d: %w", id, err)
	}
	return nil
}

// GetQuotesPaginated returns a page of quotes ordered by creation time descending.
func GetQuotesPaginated(db *sql.DB, limit, offset int) ([]Quote, error) {
	rows, err := db.Query(
		"SELECT "+quoteSelectColumns+quoteFromJoins+" ORDER BY q.created_at DESC LIMIT ? OFFSET ?",
		limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query quotes: %w", err)
	}
	defer rows.Close()

	var quotes []Quote
	for rows.Next() {
		q, err := scanQuote(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan quote: %w", err)
		}
		quotes = append(quotes, *q)
	}
	return quotes, rows.Err()
}

// CountQuotes returns the total number of quotes in the database.
func CountQuotes(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM quotes").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count quotes: %w", err)
	}
	return count, nil
}

// CountQuotesByStatus returns the number of quotes with the given status value.
func CountQuotesByStatus(db *sql.DB, status string) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM quotes WHERE status = ?", status).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count quotes by status %q: %w", status, err)
	}
	return count, nil
}

// UpdateQuoteStatus sets the status column and the appropriate timestamp column
// (sent_at, accepted_at, rejected_at) based on the new status value.
func UpdateQuoteStatus(db *sql.DB, id int, status string) error {
	var query string
	switch status {
	case "sent":
		query = "UPDATE quotes SET status = ?, sent_at = datetime('now'), updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	case "accepted":
		query = "UPDATE quotes SET status = ?, accepted_at = datetime('now'), updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	case "rejected":
		query = "UPDATE quotes SET status = ?, rejected_at = datetime('now'), updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	default:
		query = "UPDATE quotes SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?"
	}
	_, err := db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update quote %d status to %q: %w", id, status, err)
	}
	return nil
}

// GetQuoteAddons returns all add-ons attached to a quote, joined with the addons
// table to include the human-readable addon name.
func GetQuoteAddons(db *sql.DB, quoteID int) ([]QuoteAddon, error) {
	rows, err := db.Query(
		`SELECT qa.id, qa.quote_id, qa.addon_id, qa.quantity, qa.price, qa.price_type,
		        COALESCE(a.name,'')
		 FROM quote_addons qa
		 LEFT JOIN addons a ON qa.addon_id = a.id
		 WHERE qa.quote_id = ?
		 ORDER BY qa.id ASC`,
		quoteID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query addons for quote %d: %w", quoteID, err)
	}
	defer rows.Close()

	var addons []QuoteAddon
	for rows.Next() {
		var qa QuoteAddon
		if err := rows.Scan(
			&qa.ID, &qa.QuoteID, &qa.AddonID, &qa.Quantity, &qa.Price, &qa.PriceType, &qa.AddonName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan quote addon: %w", err)
		}
		addons = append(addons, qa)
	}
	return addons, rows.Err()
}

// AddQuoteAddon inserts a single addon line into quote_addons and sets its ID.
func AddQuoteAddon(db *sql.DB, qa *QuoteAddon) error {
	result, err := db.Exec(
		"INSERT INTO quote_addons (quote_id, addon_id, quantity, price, price_type) VALUES (?, ?, ?, ?, ?)",
		qa.QuoteID, qa.AddonID, qa.Quantity, qa.Price, qa.PriceType,
	)
	if err != nil {
		return fmt.Errorf("failed to add quote addon: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	qa.ID = int(id)
	return nil
}

// RemoveQuoteAddons deletes all addon lines for the given quote. Typically called
// before re-inserting a fresh set of addons on an update operation.
func RemoveQuoteAddons(db *sql.DB, quoteID int) error {
	_, err := db.Exec("DELETE FROM quote_addons WHERE quote_id = ?", quoteID)
	if err != nil {
		return fmt.Errorf("failed to remove addons for quote %d: %w", quoteID, err)
	}
	return nil
}

// CalculateQuoteTotals is a pure function that computes subtotal, tax_amount,
// and total from the pricing fields already set on q. It reads Addons to include
// any addon costs in the subtotal.
//
// Subtotal breakdown:
//   - setup_fee (one-time)
//   - domain_price (one-time)
//   - one_time addons: price * quantity
//   - recurring component: monthly_price or yearly_price based on billing_cycle
//   - monthly addons: price * quantity (treated as one billing period)
//
// Discount is applied to the subtotal before tax.
func CalculateQuoteTotals(q *Quote) {
	var addonOneTime float64
	var addonRecurring float64
	for _, a := range q.Addons {
		cost := a.Price * float64(a.Quantity)
		if a.PriceType == "monthly" {
			addonRecurring += cost
		} else {
			addonOneTime += cost
		}
	}

	var recurringBase float64
	if q.BillingCycle == "yearly" {
		recurringBase = q.YearlyPrice
	} else {
		recurringBase = q.MonthlyPrice
	}

	q.Subtotal = q.SetupFee + q.DomainPrice + addonOneTime + recurringBase + addonRecurring

	discount := q.Subtotal * (q.DiscountPercent / 100)
	q.TaxAmount = (q.Subtotal - discount) * (q.TaxRate / 100)
	q.Total = q.Subtotal - discount + q.TaxAmount
}

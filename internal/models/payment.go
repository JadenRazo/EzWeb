package models

import (
	"database/sql"
	"fmt"
)

type Payment struct {
	ID         int
	CustomerID int
	SiteID     sql.NullInt64
	Amount     float64
	DueDate    string
	PaidAt     sql.NullTime
	Status     string
	Notes      string
	CreatedAt  string

	// Display fields populated via JOIN
	CustomerName string
	SiteDomain   string
}

// SiteDropdown is a lightweight struct for dropdown population.
type SiteDropdown struct {
	ID     int
	Domain string
}

const paymentSelectColumns = `
	p.id, p.customer_id, p.site_id, p.amount, p.due_date, p.paid_at, p.notes, p.created_at,
	COALESCE(c.name, '') as customer_name,
	COALESCE(s.domain, '') as site_domain,
	CASE
		WHEN p.paid_at IS NOT NULL THEN 'paid'
		WHEN p.due_date < date('now') AND p.paid_at IS NULL THEN 'overdue'
		WHEN p.due_date <= date('now', '+7 days') AND p.paid_at IS NULL THEN 'due_soon'
		ELSE 'pending'
	END as computed_status
`

const paymentFromJoins = `
	FROM payments p
	LEFT JOIN customers c ON p.customer_id = c.id
	LEFT JOIN sites s ON p.site_id = s.id
`

func scanPayment(row interface{ Scan(dest ...any) error }) (*Payment, error) {
	var p Payment
	err := row.Scan(
		&p.ID, &p.CustomerID, &p.SiteID, &p.Amount, &p.DueDate, &p.PaidAt, &p.Notes, &p.CreatedAt,
		&p.CustomerName, &p.SiteDomain, &p.Status,
	)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func GetAllPayments(db *sql.DB) ([]Payment, error) {
	query := "SELECT " + paymentSelectColumns + paymentFromJoins + " ORDER BY p.due_date ASC"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query payments: %w", err)
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}
		payments = append(payments, *p)
	}
	return payments, rows.Err()
}

func GetPaymentByID(db *sql.DB, id int) (*Payment, error) {
	query := "SELECT " + paymentSelectColumns + paymentFromJoins + " WHERE p.id = ?"
	p, err := scanPayment(db.QueryRow(query, id))
	if err != nil {
		return nil, fmt.Errorf("payment not found: %w", err)
	}
	return p, nil
}

func GetPaymentsByCustomerID(db *sql.DB, customerID int) ([]Payment, error) {
	query := "SELECT " + paymentSelectColumns + paymentFromJoins + " WHERE p.customer_id = ? ORDER BY p.due_date ASC"
	rows, err := db.Query(query, customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query payments for customer %d: %w", customerID, err)
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}
		payments = append(payments, *p)
	}
	return payments, rows.Err()
}

func CreatePayment(db *sql.DB, p *Payment) error {
	result, err := db.Exec(
		"INSERT INTO payments (customer_id, site_id, amount, due_date, notes) VALUES (?, ?, ?, ?, ?)",
		p.CustomerID, p.SiteID, p.Amount, p.DueDate, p.Notes,
	)
	if err != nil {
		return fmt.Errorf("failed to create payment: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	p.ID = int(id)
	return nil
}

func UpdatePayment(db *sql.DB, p *Payment) error {
	_, err := db.Exec(
		"UPDATE payments SET customer_id = ?, site_id = ?, amount = ?, due_date = ?, notes = ? WHERE id = ?",
		p.CustomerID, p.SiteID, p.Amount, p.DueDate, p.Notes, p.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update payment: %w", err)
	}
	return nil
}

func MarkPaymentPaid(db *sql.DB, id int) error {
	_, err := db.Exec(
		"UPDATE payments SET paid_at = datetime('now'), status = 'paid' WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to mark payment as paid: %w", err)
	}
	return nil
}

func DeletePayment(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM payments WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete payment: %w", err)
	}
	return nil
}

func CountOverduePayments(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow(
		"SELECT COUNT(*) FROM payments WHERE paid_at IS NULL AND due_date < date('now')",
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count overdue payments: %w", err)
	}
	return count, nil
}

func CountPayments(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM payments").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count payments: %w", err)
	}
	return count, nil
}

func GetPaymentsPaginated(db *sql.DB, limit, offset int) ([]Payment, error) {
	query := "SELECT " + paymentSelectColumns + paymentFromJoins + " ORDER BY p.due_date ASC LIMIT ? OFFSET ?"
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query payments: %w", err)
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan payment: %w", err)
		}
		payments = append(payments, *p)
	}
	return payments, rows.Err()
}

func GetSitesForDropdown(db *sql.DB) ([]SiteDropdown, error) {
	rows, err := db.Query("SELECT id, domain FROM sites ORDER BY domain ASC")
	if err != nil {
		return nil, fmt.Errorf("failed to query sites: %w", err)
	}
	defer rows.Close()

	var sites []SiteDropdown
	for rows.Next() {
		var s SiteDropdown
		if err := rows.Scan(&s.ID, &s.Domain); err != nil {
			return nil, fmt.Errorf("failed to scan site: %w", err)
		}
		sites = append(sites, s)
	}
	return sites, rows.Err()
}

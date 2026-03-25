package models

import (
	"database/sql"
	"fmt"
	"time"
)

// Subscription represents a recurring billing agreement tied to a customer and
// optionally a specific managed site.
type Subscription struct {
	ID                  int
	CustomerID          int
	SiteID              sql.NullInt64
	Amount              float64
	BillingCycle        string
	NextDueDate         string
	Status              string
	AutoGenerateInvoice bool
	CreatedAt           time.Time
	UpdatedAt           time.Time

	// Populated via JOIN
	CustomerName string
	SiteDomain   string
}

const subscriptionSelectColumns = `
	s.id, s.customer_id, s.site_id, s.amount, s.billing_cycle, s.next_due_date,
	s.status, COALESCE(s.auto_generate_invoice, 1),
	s.created_at, s.updated_at,
	COALESCE(c.name, '') as customer_name,
	COALESCE(si.domain, '') as site_domain`

const subscriptionFromJoins = `
	FROM subscriptions s
	LEFT JOIN customers c ON s.customer_id = c.id
	LEFT JOIN sites si ON s.site_id = si.id`

func scanSubscription(scanner interface{ Scan(dest ...any) error }) (*Subscription, error) {
	var sub Subscription
	var autoInt int
	err := scanner.Scan(
		&sub.ID, &sub.CustomerID, &sub.SiteID, &sub.Amount, &sub.BillingCycle, &sub.NextDueDate,
		&sub.Status, &autoInt,
		&sub.CreatedAt, &sub.UpdatedAt,
		&sub.CustomerName, &sub.SiteDomain,
	)
	if err != nil {
		return nil, err
	}
	sub.AutoGenerateInvoice = autoInt == 1
	return &sub, nil
}

func GetAllSubscriptions(db *sql.DB) ([]Subscription, error) {
	query := "SELECT " + subscriptionSelectColumns + subscriptionFromJoins + " ORDER BY s.next_due_date ASC"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

func GetSubscriptionsByCustomerID(db *sql.DB, customerID int) ([]Subscription, error) {
	query := "SELECT " + subscriptionSelectColumns + subscriptionFromJoins + " WHERE s.customer_id = ? ORDER BY s.next_due_date ASC"
	rows, err := db.Query(query, customerID)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions for customer %d: %w", customerID, err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

func GetSubscriptionByID(db *sql.DB, id int) (*Subscription, error) {
	query := "SELECT " + subscriptionSelectColumns + subscriptionFromJoins + " WHERE s.id = ?"
	sub, err := scanSubscription(db.QueryRow(query, id))
	if err != nil {
		return nil, fmt.Errorf("subscription not found: %w", err)
	}
	return sub, nil
}

func CreateSubscription(db *sql.DB, sub *Subscription) error {
	autoInt := 1
	if !sub.AutoGenerateInvoice {
		autoInt = 0
	}
	result, err := db.Exec(
		`INSERT INTO subscriptions (customer_id, site_id, amount, billing_cycle, next_due_date, status, auto_generate_invoice)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sub.CustomerID, sub.SiteID, sub.Amount, sub.BillingCycle, sub.NextDueDate, sub.Status, autoInt,
	)
	if err != nil {
		return fmt.Errorf("failed to create subscription: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	sub.ID = int(id)
	return nil
}

func UpdateSubscription(db *sql.DB, sub *Subscription) error {
	autoInt := 0
	if sub.AutoGenerateInvoice {
		autoInt = 1
	}
	_, err := db.Exec(
		`UPDATE subscriptions
		 SET customer_id = ?, site_id = ?, amount = ?, billing_cycle = ?,
		     next_due_date = ?, status = ?, auto_generate_invoice = ?,
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		sub.CustomerID, sub.SiteID, sub.Amount, sub.BillingCycle,
		sub.NextDueDate, sub.Status, autoInt, sub.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update subscription %d: %w", sub.ID, err)
	}
	return nil
}

func PauseSubscription(db *sql.DB, id int) error {
	_, err := db.Exec(
		"UPDATE subscriptions SET status = 'paused', updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to pause subscription %d: %w", id, err)
	}
	return nil
}

func CancelSubscription(db *sql.DB, id int) error {
	_, err := db.Exec(
		"UPDATE subscriptions SET status = 'cancelled', updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		id,
	)
	if err != nil {
		return fmt.Errorf("failed to cancel subscription %d: %w", id, err)
	}
	return nil
}

// GetDueSubscriptions returns all active subscriptions whose next_due_date is
// today or in the past. Used by the billing runner to generate invoices.
func GetDueSubscriptions(db *sql.DB) ([]Subscription, error) {
	query := "SELECT " + subscriptionSelectColumns + subscriptionFromJoins +
		" WHERE s.status = 'active' AND s.next_due_date <= date('now') ORDER BY s.next_due_date ASC"
	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query due subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

// AdvanceSubscriptionDueDate moves next_due_date forward by one billing period.
func AdvanceSubscriptionDueDate(db *sql.DB, id int) error {
	sub, err := GetSubscriptionByID(db, id)
	if err != nil {
		return fmt.Errorf("subscription not found for advancement: %w", err)
	}

	current, err := time.Parse("2006-01-02", sub.NextDueDate)
	if err != nil {
		return fmt.Errorf("failed to parse next_due_date %q: %w", sub.NextDueDate, err)
	}

	var next time.Time
	switch sub.BillingCycle {
	case "yearly":
		next = current.AddDate(1, 0, 0)
	default: // monthly
		next = current.AddDate(0, 1, 0)
	}

	_, err = db.Exec(
		"UPDATE subscriptions SET next_due_date = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
		next.Format("2006-01-02"), id,
	)
	if err != nil {
		return fmt.Errorf("failed to advance due date for subscription %d: %w", id, err)
	}
	return nil
}

func CountSubscriptions(db *sql.DB) (int, error) {
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM subscriptions").Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count subscriptions: %w", err)
	}
	return count, nil
}

func GetSubscriptionsPaginated(db *sql.DB, limit, offset int) ([]Subscription, error) {
	query := "SELECT " + subscriptionSelectColumns + subscriptionFromJoins +
		" ORDER BY s.next_due_date ASC LIMIT ? OFFSET ?"
	rows, err := db.Query(query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %w", err)
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription: %w", err)
		}
		subs = append(subs, *sub)
	}
	return subs, rows.Err()
}

func DeleteSubscription(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM subscriptions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete subscription %d: %w", id, err)
	}
	return nil
}

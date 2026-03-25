package models

import (
	"database/sql"
	"fmt"
	"strconv"
)

// GetSetting returns the value for the given key from business_settings.
// Returns an empty string if the key does not exist.
func GetSetting(db *sql.DB, key string) string {
	var value string
	err := db.QueryRow("SELECT value FROM business_settings WHERE key = ?", key).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

// SetSetting upserts a single key/value pair in business_settings.
func SetSetting(db *sql.DB, key, value string) error {
	_, err := db.Exec(
		`INSERT INTO business_settings (key, value, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to upsert setting %q: %w", key, err)
	}
	return nil
}

// GetAllSettings returns every row in business_settings as a map[key]value.
func GetAllSettings(db *sql.DB) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM business_settings")
	if err != nil {
		return nil, fmt.Errorf("failed to query business_settings: %w", err)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("failed to scan setting row: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetSettings bulk-upserts the supplied map inside a single transaction.
func SetSettings(db *sql.DB, settings map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin settings transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(
		`INSERT INTO business_settings (key, value, updated_at)
		 VALUES (?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
	)
	if err != nil {
		return fmt.Errorf("failed to prepare settings statement: %w", err)
	}
	defer stmt.Close()

	for k, v := range settings {
		if _, err := stmt.Exec(k, v); err != nil {
			return fmt.Errorf("failed to upsert setting %q: %w", k, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit settings transaction: %w", err)
	}
	return nil
}

// Typed helpers — each reads a single well-known key with a sensible zero value.

func GetBusinessName(db *sql.DB) string    { return GetSetting(db, "business_name") }
func GetLogoPath(db *sql.DB) string        { return GetSetting(db, "logo_path") }
func GetBusinessEmail(db *sql.DB) string   { return GetSetting(db, "email") }
func GetBusinessPhone(db *sql.DB) string   { return GetSetting(db, "phone") }
func GetBusinessAddress(db *sql.DB) string { return GetSetting(db, "address") }
func GetTagline(db *sql.DB) string         { return GetSetting(db, "tagline") }
func GetWebsiteURL(db *sql.DB) string      { return GetSetting(db, "website_url") }
func GetDefaultCurrency(db *sql.DB) string { return GetSetting(db, "default_currency") }
func GetTermsText(db *sql.DB) string       { return GetSetting(db, "terms_text") }

// GetTaxRate returns the tax_rate setting as a float64. Returns 0 on parse error.
func GetTaxRate(db *sql.DB) float64 {
	v := GetSetting(db, "tax_rate")
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return f
}

// GetQuoteValidityDays returns the quote_validity_days setting as an int. Returns 30 on parse error.
func GetQuoteValidityDays(db *sql.DB) int {
	v := GetSetting(db, "quote_validity_days")
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 30
	}
	return n
}

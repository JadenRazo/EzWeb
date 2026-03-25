package models

import (
	"database/sql"
	"fmt"
	"time"
)

type PricingTier struct {
	ID           int
	TemplateSlug string
	Label        string
	SetupFee     float64
	MonthlyPrice float64
	YearlyPrice  float64
	Description  string
	IsActive     bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Addon struct {
	ID          int
	Name        string
	Description string
	Price       float64
	PriceType   string // "one_time" or "monthly"
	IsActive    bool
	CreatedAt   time.Time
}

func scanPricingTier(scanner interface{ Scan(dest ...any) error }) (*PricingTier, error) {
	var t PricingTier
	var isActiveInt int
	err := scanner.Scan(
		&t.ID, &t.TemplateSlug, &t.Label, &t.SetupFee, &t.MonthlyPrice, &t.YearlyPrice,
		&t.Description, &isActiveInt, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.IsActive = isActiveInt == 1
	return &t, nil
}

const pricingTierSelectColumns = `
	id, template_slug, label, setup_fee, monthly_price, yearly_price,
	COALESCE(description,''), COALESCE(is_active,1), created_at, updated_at`

// GetAllPricingTiers returns all active pricing tiers ordered by setup fee ascending.
func GetAllPricingTiers(db *sql.DB) ([]PricingTier, error) {
	rows, err := db.Query(
		"SELECT" + pricingTierSelectColumns + " FROM pricing_tiers WHERE is_active = 1 ORDER BY setup_fee ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query pricing tiers: %w", err)
	}
	defer rows.Close()

	var tiers []PricingTier
	for rows.Next() {
		t, err := scanPricingTier(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan pricing tier: %w", err)
		}
		tiers = append(tiers, *t)
	}
	return tiers, rows.Err()
}

// GetPricingTierBySlug returns the pricing tier matching the given template slug.
func GetPricingTierBySlug(db *sql.DB, slug string) (*PricingTier, error) {
	row := db.QueryRow(
		"SELECT"+pricingTierSelectColumns+" FROM pricing_tiers WHERE template_slug = ?",
		slug,
	)
	t, err := scanPricingTier(row)
	if err != nil {
		return nil, fmt.Errorf("pricing tier not found for slug %q: %w", slug, err)
	}
	return t, nil
}

// GetActivePricingTiers returns all active pricing tiers ordered by setup fee ascending.
// Alias for GetAllPricingTiers kept for semantic clarity at call sites.
func GetActivePricingTiers(db *sql.DB) ([]PricingTier, error) {
	return GetAllPricingTiers(db)
}

// UpdatePricingTier writes updated fields back to the database for an existing tier.
func UpdatePricingTier(db *sql.DB, t *PricingTier) error {
	isActiveInt := 0
	if t.IsActive {
		isActiveInt = 1
	}
	_, err := db.Exec(
		`UPDATE pricing_tiers
		 SET label = ?, setup_fee = ?, monthly_price = ?, yearly_price = ?,
		     description = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		 WHERE template_slug = ?`,
		t.Label, t.SetupFee, t.MonthlyPrice, t.YearlyPrice,
		t.Description, isActiveInt, t.TemplateSlug,
	)
	if err != nil {
		return fmt.Errorf("failed to update pricing tier %q: %w", t.TemplateSlug, err)
	}
	return nil
}

func scanAddon(scanner interface{ Scan(dest ...any) error }) (*Addon, error) {
	var a Addon
	var isActiveInt int
	err := scanner.Scan(
		&a.ID, &a.Name, &a.Description, &a.Price, &a.PriceType, &isActiveInt, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.IsActive = isActiveInt == 1
	return &a, nil
}

const addonSelectColumns = `
	id, name, COALESCE(description,''), price, price_type, COALESCE(is_active,1), created_at`

// GetAllAddons returns all active add-ons ordered by name.
func GetAllAddons(db *sql.DB) ([]Addon, error) {
	rows, err := db.Query(
		"SELECT" + addonSelectColumns + " FROM addons WHERE is_active = 1 ORDER BY name ASC",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query addons: %w", err)
	}
	defer rows.Close()

	var addons []Addon
	for rows.Next() {
		a, err := scanAddon(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan addon: %w", err)
		}
		addons = append(addons, *a)
	}
	return addons, rows.Err()
}

// GetAddonByID returns the addon with the given primary key.
func GetAddonByID(db *sql.DB, id int) (*Addon, error) {
	row := db.QueryRow("SELECT"+addonSelectColumns+" FROM addons WHERE id = ?", id)
	a, err := scanAddon(row)
	if err != nil {
		return nil, fmt.Errorf("addon not found: %w", err)
	}
	return a, nil
}

// CreateAddon inserts a new addon and sets its ID on the struct.
func CreateAddon(db *sql.DB, a *Addon) error {
	isActiveInt := 1
	if !a.IsActive {
		isActiveInt = 0
	}
	result, err := db.Exec(
		"INSERT INTO addons (name, description, price, price_type, is_active) VALUES (?, ?, ?, ?, ?)",
		a.Name, a.Description, a.Price, a.PriceType, isActiveInt,
	)
	if err != nil {
		return fmt.Errorf("failed to create addon: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert id: %w", err)
	}
	a.ID = int(id)
	return nil
}

// UpdateAddon writes updated fields back to the database for an existing addon.
func UpdateAddon(db *sql.DB, a *Addon) error {
	isActiveInt := 0
	if a.IsActive {
		isActiveInt = 1
	}
	_, err := db.Exec(
		"UPDATE addons SET name = ?, description = ?, price = ?, price_type = ?, is_active = ? WHERE id = ?",
		a.Name, a.Description, a.Price, a.PriceType, isActiveInt, a.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update addon %d: %w", a.ID, err)
	}
	return nil
}

// DeleteAddon removes an addon by ID.
func DeleteAddon(db *sql.DB, id int) error {
	_, err := db.Exec("DELETE FROM addons WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete addon %d: %w", id, err)
	}
	return nil
}

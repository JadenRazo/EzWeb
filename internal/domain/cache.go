package domain

import (
	"database/sql"
	"log"
	"time"
)

const (
	cacheTTL     = 5 * time.Minute
	cacheMaxAge  = 24 * time.Hour
)

// Cache wraps the domain_price_cache SQLite table to avoid hammering registrar
// APIs on repeated searches for the same domain.
type Cache struct {
	db *sql.DB
}

// NewCache returns a Cache backed by the given database connection.
func NewCache(db *sql.DB) *Cache {
	return &Cache{db: db}
}

// Get returns the cached DomainResult for the given domain + provider pair if
// the entry exists and was written within the last 5 minutes.  The second
// return value is false when no fresh entry was found.
func (c *Cache) Get(domain, provider string) (*DomainResult, bool) {
	row := c.db.QueryRow(
		`SELECT domain, tld, provider, available, register_price, renew_price, cached_at
		 FROM domain_price_cache
		 WHERE domain = ? AND provider = ?
		 ORDER BY cached_at DESC
		 LIMIT 1`,
		domain, provider,
	)

	var (
		r        DomainResult
		avail    int
		cachedAt time.Time
	)

	err := row.Scan(&r.Domain, &r.TLD, &r.Provider, &avail, &r.RegisterPrice, &r.RenewPrice, &cachedAt)
	if err != nil {
		// No row or scan error — treat as cache miss.
		return nil, false
	}

	if time.Since(cachedAt) > cacheTTL {
		return nil, false
	}

	r.Available = avail == 1
	r.Currency = "USD"
	return &r, true
}

// Set upserts a DomainResult into the cache table.  Existing entries for the
// same (domain, provider) pair are replaced so the table does not grow
// unboundedly between CleanExpired runs.
func (c *Cache) Set(result *DomainResult) error {
	avail := 0
	if result.Available {
		avail = 1
	}

	_, err := c.db.Exec(
		`INSERT INTO domain_price_cache (domain, tld, provider, available, register_price, renew_price, cached_at)
		 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		 ON CONFLICT DO NOTHING`,
		result.Domain, result.TLD, result.Provider, avail, result.RegisterPrice, result.RenewPrice,
	)
	if err != nil {
		// SQLite does not support ON CONFLICT on non-UNIQUE columns; fall back
		// to a DELETE + INSERT pattern if the above fails.
		if _, delErr := c.db.Exec(
			`DELETE FROM domain_price_cache WHERE domain = ? AND provider = ?`,
			result.Domain, result.Provider,
		); delErr != nil {
			return delErr
		}
		_, err = c.db.Exec(
			`INSERT INTO domain_price_cache (domain, tld, provider, available, register_price, renew_price, cached_at)
			 VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
			result.Domain, result.TLD, result.Provider, avail, result.RegisterPrice, result.RenewPrice,
		)
	}
	return err
}

// CleanExpired removes cache entries older than 24 hours.  It is safe to call
// this in the background and errors are logged rather than returned.
func (c *Cache) CleanExpired() error {
	cutoff := time.Now().Add(-cacheMaxAge).UTC().Format("2006-01-02 15:04:05")
	res, err := c.db.Exec(
		`DELETE FROM domain_price_cache WHERE cached_at < ?`, cutoff,
	)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		log.Printf("domain cache: pruned %d expired entries", n)
	}
	return nil
}

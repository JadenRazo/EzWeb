package auth

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// revocationCache holds a short-lived in-memory cache of token revocation
// lookups, reducing per-request DB queries for the hot auth path.
//
// Positive results (token is revoked) are cached for revokedTTL.
// Negative results (token is not revoked) are cached for notRevokedTTL.
// The shorter TTL for negative results limits the window where a token that
// was just revoked could slip through on another request before the cache
// entry expires.
type revocationCacheStore struct {
	mu      sync.RWMutex
	revoked map[string]bool
	expiry  map[string]time.Time
}

const (
	revokedTTL    = 5 * time.Minute
	notRevokedTTL = 1 * time.Minute
)

// globalRevocationCache is the package-level singleton used by IsRevoked and
// RevokeToken. It requires no explicit initialisation; the zero value is ready.
var globalRevocationCache = &revocationCacheStore{
	revoked: make(map[string]bool),
	expiry:  make(map[string]time.Time),
}

// get returns (result, ok). ok is false when the entry is absent or expired.
func (c *revocationCacheStore) get(jti string) (revoked bool, ok bool) {
	c.mu.RLock()
	exp, exists := c.expiry[jti]
	if !exists {
		c.mu.RUnlock()
		return false, false
	}
	if time.Now().After(exp) {
		c.mu.RUnlock()
		// Entry expired — promote to write lock for cleanup.
		c.evict(jti)
		return false, false
	}
	rev := c.revoked[jti]
	c.mu.RUnlock()
	return rev, true
}

// set stores a result for jti with the appropriate TTL.
func (c *revocationCacheStore) set(jti string, revoked bool) {
	ttl := notRevokedTTL
	if revoked {
		ttl = revokedTTL
	}
	c.mu.Lock()
	c.revoked[jti] = revoked
	c.expiry[jti] = time.Now().Add(ttl)
	c.mu.Unlock()
}

// evict removes a single expired entry under a write lock.
func (c *revocationCacheStore) evict(jti string) {
	c.mu.Lock()
	delete(c.revoked, jti)
	delete(c.expiry, jti)
	c.mu.Unlock()
}

type Claims struct {
	UserID   int    `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func GenerateToken(userID int, username, role, secret string, expiryHours int) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.NewString(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expiryHours) * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}
	return signed, nil
}

func ValidateToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}
	return claims, nil
}

// RevokeToken inserts the token's JTI into the blocklist so it cannot be
// reused, and immediately records the revocation in the in-memory cache so
// that the same process sees the change without waiting for a DB round-trip.
func RevokeToken(db *sql.DB, jti string, expiresAt time.Time) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO revoked_tokens (jti, expires_at) VALUES (?, ?)",
		jti, expiresAt,
	)
	if err == nil && jti != "" {
		globalRevocationCache.set(jti, true)
	}
	return err
}

// IsRevoked checks whether a token JTI has been revoked. It consults the
// in-memory cache first and only falls back to the database on a cache miss,
// which dramatically reduces read pressure on the DB for the hot auth path.
func IsRevoked(db *sql.DB, jti string) bool {
	if jti == "" {
		return false
	}

	// Fast path: cache hit.
	if revoked, ok := globalRevocationCache.get(jti); ok {
		return revoked
	}

	// Slow path: query the database and populate the cache.
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM revoked_tokens WHERE jti = ?", jti).Scan(&count)
	if err != nil {
		// On DB error, fail open (allow the request) to avoid locking out
		// users during transient DB issues. The token will be re-checked on
		// the next request.
		return false
	}

	revoked := count > 0
	globalRevocationCache.set(jti, revoked)
	return revoked
}

// CleanupExpiredTokens removes revoked token entries that have already expired.
func CleanupExpiredTokens(db *sql.DB) {
	db.Exec("DELETE FROM revoked_tokens WHERE expires_at < ?", time.Now().UTC().Format(time.RFC3339))
}

package auth

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

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

// RevokeToken inserts the token's JTI into the blocklist so it cannot be reused.
func RevokeToken(db *sql.DB, jti string, expiresAt time.Time) error {
	_, err := db.Exec(
		"INSERT OR IGNORE INTO revoked_tokens (jti, expires_at) VALUES (?, ?)",
		jti, expiresAt,
	)
	return err
}

// IsRevoked checks whether a token JTI has been revoked.
func IsRevoked(db *sql.DB, jti string) bool {
	if jti == "" {
		return false
	}
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM revoked_tokens WHERE jti = ?", jti).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// CleanupExpiredTokens removes revoked token entries that have already expired.
func CleanupExpiredTokens(db *sql.DB) {
	db.Exec("DELETE FROM revoked_tokens WHERE expires_at < ?", time.Now().UTC().Format(time.RFC3339))
}

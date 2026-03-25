package portal

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"time"

	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
)

// GenerateMagicToken creates a cryptographically random 32-byte token. It
// returns the plain hex string (sent to the user via email/log) and the
// SHA-256 hash of that string (stored in the database).
func GenerateMagicToken() (plainToken string, hashedToken string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plainToken = hex.EncodeToString(b)
	hashedToken = HashToken(plainToken)
	return plainToken, hashedToken, nil
}

// HashToken returns the SHA-256 hex digest of a plain token. Used at
// verification time to look up the stored hash from the database.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// ClientAuthMiddleware validates the client_token cookie against the
// client_tokens table. On success it stores the customer_id in Fiber locals
// so downstream handlers can read it with c.Locals("customer_id").(int).
// On failure it redirects to /portal/login.
func ClientAuthMiddleware(db *sql.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		cookie := c.Cookies("client_token")
		if cookie == "" {
			return c.Redirect("/portal/login")
		}

		hash := HashToken(cookie)
		token, err := models.GetClientTokenByHash(db, hash)
		if err != nil {
			// Token not found — clear the stale cookie and redirect.
			c.Cookie(&fiber.Cookie{
				Name:     "client_token",
				Value:    "",
				Expires:  time.Unix(0, 0),
				HTTPOnly: true,
				SameSite: "Lax",
				Path:     "/",
			})
			return c.Redirect("/portal/login")
		}

		if time.Now().After(token.ExpiresAt) {
			// Expired — clean up and redirect.
			_ = models.DeleteClientToken(db, token.ID)
			c.Cookie(&fiber.Cookie{
				Name:     "client_token",
				Value:    "",
				Expires:  time.Unix(0, 0),
				HTTPOnly: true,
				SameSite: "Lax",
				Path:     "/",
			})
			return c.Redirect("/portal/login")
		}

		c.Locals("customer_id", token.CustomerID)
		return c.Next()
	}
}

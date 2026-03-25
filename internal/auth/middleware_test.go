package auth

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	_ "modernc.org/sqlite"
)

const testSecret = "test-secret-key-at-least-32-chars!!"

// newTestDB opens an in-memory SQLite database and creates the revoked_tokens
// table so revocation checks have a valid schema to operate against.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open in-memory db: %v", err)
	}
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS revoked_tokens (jti TEXT PRIMARY KEY, expires_at DATETIME)`)
	if err != nil {
		t.Fatalf("create revoked_tokens table: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

// newApp builds a minimal Fiber app wired through the supplied middleware chain
// and returns 200 OK with body "ok" if every handler calls Next successfully.
func newApp(middleware ...fiber.Handler) *fiber.App {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	handlers := append(middleware, func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})
	app.Get("/", handlers...)
	app.Post("/", handlers...)
	return app
}

// tokenCookie builds a Cookie header string from a signed JWT so app.Test()
// can attach it to a synthetic request.
func tokenCookie(t *testing.T, userID int, username, role string) string {
	t.Helper()
	tok, err := GenerateToken(userID, username, role, testSecret, 24)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	return "token=" + tok
}

// --- AuthMiddleware tests ---

func TestAuthMiddleware_RedirectsWhenNoCookie(t *testing.T) {
	app := newApp(AuthMiddleware(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestAuthMiddleware_RedirectsWhenTokenInvalid(t *testing.T) {
	app := newApp(AuthMiddleware(testSecret))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", "token=this.is.not.a.valid.jwt")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 on invalid token, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestAuthMiddleware_SetsLocalsAndCallsNext(t *testing.T) {
	var (
		capturedUserID   interface{}
		capturedUsername interface{}
		capturedRole     interface{}
		capturedClaims   interface{}
	)

	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", AuthMiddleware(testSecret), func(c *fiber.Ctx) error {
		capturedUserID = c.Locals("user_id")
		capturedUsername = c.Locals("username")
		capturedRole = c.Locals("role")
		capturedClaims = c.Locals("token_claims")
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", tokenCookie(t, 42, "jaden", "admin"))

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if uid, ok := capturedUserID.(int); !ok || uid != 42 {
		t.Errorf("user_id local: expected 42, got %v", capturedUserID)
	}
	if uname, ok := capturedUsername.(string); !ok || uname != "jaden" {
		t.Errorf("username local: expected \"jaden\", got %v", capturedUsername)
	}
	if role, ok := capturedRole.(string); !ok || role != "admin" {
		t.Errorf("role local: expected \"admin\", got %v", capturedRole)
	}
	if capturedClaims == nil {
		t.Error("token_claims local should not be nil")
	}
}

func TestAuthMiddleware_RedirectsWhenTokenRevoked(t *testing.T) {
	db := newTestDB(t)

	tok, err := GenerateToken(1, "jaden", "admin", testSecret, 24)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	claims, err := ValidateToken(tok, testSecret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if err = RevokeToken(db, claims.ID, time.Now().Add(24*time.Hour)); err != nil {
		t.Fatalf("RevokeToken: %v", err)
	}

	app := newApp(AuthMiddleware(testSecret, db))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Cookie", "token="+tok)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusFound {
		t.Errorf("expected 302 for revoked token, got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// --- AdminOnly tests ---

func TestAdminOnly_AllowsAdminRole(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		return c.Next()
	}, AdminOnly(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for admin role, got %d", resp.StatusCode)
	}
}

func TestAdminOnly_BlocksNonAdminRole(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("role", "viewer")
		return c.Next()
	}, AdminOnly(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for non-admin role, got %d", resp.StatusCode)
	}
}

// --- WriteProtect tests ---

func TestWriteProtect_AllowsGETForNonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Get("/", func(c *fiber.Ctx) error {
		c.Locals("role", "viewer")
		return c.Next()
	}, WriteProtect(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for GET with non-admin, got %d", resp.StatusCode)
	}
}

func TestWriteProtect_BlocksPOSTForNonAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/", func(c *fiber.Ctx) error {
		c.Locals("role", "viewer")
		return c.Next()
	}, WriteProtect(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("expected 403 for POST with non-admin, got %d", resp.StatusCode)
	}
}

func TestWriteProtect_AllowsPOSTForAdmin(t *testing.T) {
	app := fiber.New(fiber.Config{DisableStartupMessage: true})
	app.Post("/", func(c *fiber.Ctx) error {
		c.Locals("role", "admin")
		return c.Next()
	}, WriteProtect(), func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 for POST with admin, got %d", resp.StatusCode)
	}
}

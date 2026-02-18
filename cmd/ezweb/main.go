package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ezweb/internal/auth"
	"ezweb/internal/caddy"
	"ezweb/internal/config"
	"ezweb/internal/db"
	"ezweb/internal/handlers"
	"ezweb/internal/health"
	"ezweb/internal/models"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/csrf"
	"github.com/gofiber/fiber/v2/middleware/helmet"
	"github.com/gofiber/fiber/v2/middleware/limiter"
	"github.com/gofiber/fiber/v2/middleware/recover"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	hashedPass, err := auth.HashPassword(cfg.AdminPass)
	if err != nil {
		log.Fatalf("Failed to hash admin password: %v", err)
	}

	if err := models.EnsureAdminExists(database, cfg.AdminUser, hashedPass); err != nil {
		log.Fatalf("Failed to ensure admin user: %v", err)
	}

	// Caddy manager
	caddyMgr := caddy.NewManager(cfg.CaddyfilePath)

	// Start background health checker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checker := health.NewChecker(database, 5*time.Minute, cfg.WebhookURL, cfg.WebhookFormat, cfg.AlertThreshold)
	go checker.Start(ctx)

	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			return c.Status(code).SendString(err.Error())
		},
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(helmet.New())

	// Static files
	app.Static("/static", "./static")

	// Rate limit on login
	loginLimiter := limiter.New(limiter.Config{
		Max:        10,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	})

	// Public routes
	app.Get("/login", handlers.LoginPage)
	app.Post("/login", loginLimiter, handlers.LoginPost(database, cfg))
	app.Get("/logout", handlers.Logout)

	// Protected routes
	protected := app.Group("/", auth.AuthMiddleware(cfg.JWTSecret))

	// General rate limiter for protected routes
	protected.Use(limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	}))

	// CSRF protection
	protected.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookieSameSite: "Lax",
		CookieHTTPOnly: false,
		Expiration:     1 * time.Hour,
	}))

	// Dashboard
	protected.Get("/dashboard", handlers.Dashboard(database))

	// Customer CRUD
	protected.Get("/customers", handlers.ListCustomers(database))
	protected.Post("/customers", handlers.CreateCustomer(database))
	protected.Get("/customers/:id/edit", handlers.EditCustomerForm(database))
	protected.Get("/customers/:id/cancel", handlers.CancelEditCustomer(database))
	protected.Put("/customers/:id", handlers.UpdateCustomer(database))
	protected.Delete("/customers/:id", handlers.DeleteCustomer(database))

	// Server CRUD + Test Connection
	protected.Get("/servers", handlers.ListServers(database))
	protected.Post("/servers", handlers.CreateServerHandler(database))
	protected.Get("/servers/:id/edit", handlers.EditServerForm(database))
	protected.Get("/servers/:id/row", handlers.CancelEditServer(database))
	protected.Put("/servers/:id", handlers.UpdateServerHandler(database))
	protected.Delete("/servers/:id", handlers.DeleteServerHandler(database))
	protected.Post("/servers/:id/test", handlers.TestServerConnection(database))

	// Site CRUD + Deploy/Control
	protected.Get("/sites", handlers.ListSites(database))
	protected.Get("/sites/new", handlers.CreateSiteForm(database))
	protected.Post("/sites", handlers.CreateSite(database, caddyMgr))
	protected.Get("/sites/:id", handlers.SiteDetail(database))
	protected.Put("/sites/:id", handlers.UpdateSite(database, caddyMgr))
	protected.Delete("/sites/:id", handlers.DeleteSite(database, caddyMgr))
	protected.Post("/sites/:id/deploy", handlers.DeploySite(database))
	protected.Get("/sites/:id/deploy/stream", handlers.DeploySSE(database))
	protected.Post("/sites/:id/start", handlers.StartSite(database))
	protected.Post("/sites/:id/stop", handlers.StopSite(database))
	protected.Post("/sites/:id/restart", handlers.RestartSite(database))

	// Site Logs + Health
	protected.Get("/sites/:id/logs", handlers.GetSiteLogs(database))
	protected.Get("/sites/:id/health", handlers.GetSiteHealth(database))

	// Import
	protected.Get("/import", handlers.ImportPage())
	protected.Post("/import/scan", handlers.ScanProjects(database))
	protected.Post("/import", handlers.ImportProject(database, caddyMgr))

	// Payment CRUD
	protected.Get("/payments", handlers.ListPayments(database))
	protected.Post("/payments", handlers.CreatePayment(database))
	protected.Get("/payments/:id/edit", handlers.EditPaymentForm(database))
	protected.Get("/payments/:id/row", handlers.CancelEditPayment(database))
	protected.Put("/payments/:id", handlers.UpdatePayment(database))
	protected.Post("/payments/:id/mark-paid", handlers.MarkPaid(database))
	protected.Delete("/payments/:id", handlers.DeletePayment(database))

	// Templates API
	protected.Get("/api/templates", handlers.ListTemplates(database))

	// Redirect root to dashboard
	app.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/dashboard")
	})

	// Graceful shutdown
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		<-sigChan
		log.Println("Shutting down...")
		cancel()
		_ = app.Shutdown()
	}()

	log.Printf("EzWeb starting on port %s", cfg.Port)
	log.Fatal(app.Listen(":" + cfg.Port))
}

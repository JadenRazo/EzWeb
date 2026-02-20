package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ezweb/internal/auth"
	"ezweb/internal/backup"
	"ezweb/internal/caddy"
	"ezweb/internal/config"
	"ezweb/internal/db"
	"ezweb/internal/handlers"
	"ezweb/internal/health"
	"ezweb/internal/metrics"
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

	database, err := db.Open(cfg.DBPath, cfg.DBMaxOpenConns, cfg.DBMaxIdleConns)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	// EnsureAdminExists hashes the password internally and updates the stored
	// hash when the .env password has changed since the last run.
	if err := models.EnsureAdminExists(database, cfg.AdminUser, cfg.AdminPass); err != nil {
		log.Fatalf("Failed to ensure admin user: %v", err)
	}

	// Seed activity log for pre-existing entities (no-op if already populated)
	models.BackfillActivities(database)

	// Account lockout tracker
	lockout := auth.NewLockoutTracker(cfg.LockoutMaxAttempts, time.Duration(cfg.LockoutDurationMin)*time.Minute)

	// Backup manager
	backupMgr := backup.NewManager(cfg.BackupDir, database)

	// Caddy manager
	caddyMgr := caddy.NewManager(cfg.CaddyfilePath, cfg.AcmeEmail)

	// Start background health checker
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	emailSender := health.NewEmailSender(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPFrom, cfg.AlertEmail, cfg.SMTPUsername, cfg.SMTPPassword)
	checker := health.NewChecker(database, time.Duration(cfg.HealthCheckInterval)*time.Minute, cfg.WebhookURL, cfg.WebhookFormat, cfg.AlertThreshold, cfg.HealthRetentionDays, cfg.ActivityRetentionDays, emailSender)
	go checker.Start(ctx)

	app := fiber.New(fiber.Config{
		// Trust X-Forwarded-For from local reverse proxies (e.g. Caddy) so
		// the rate limiter sees the real client IP instead of 127.0.0.1.
		ProxyHeader:    "X-Forwarded-For",
		TrustedProxies: []string{"127.0.0.1", "::1"},

		// Server-side timeouts.  WriteTimeout is generous to accommodate the
		// SSE deploy stream, which can run for several minutes.
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 10 * time.Minute,
		IdleTimeout:  60 * time.Second,

		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
			}
			log.Printf("HTTP %d: %v", code, err)
			return c.Status(code).SendString("An error occurred")
		},
	})

	// Global middleware
	app.Use(recover.New())
	app.Use(helmet.New())

	// Metrics middleware (counts requests, tracks latency)
	if cfg.MetricsEnabled {
		app.Use(metrics.Middleware())
	}

	// Static files
	app.Static("/static", "./static")

	// Health probe — unauthenticated, before any auth middleware.
	app.Get("/healthz", func(c *fiber.Ctx) error {
		if err := database.Ping(); err != nil {
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"status": "degraded",
				"db":     "unreachable",
			})
		}
		return c.JSON(fiber.Map{"status": "ok", "db": "connected"})
	})

	// Prometheus metrics endpoint (unauthenticated for scraping)
	if cfg.MetricsEnabled {
		app.Get("/metrics", metrics.Handler())
	}

	// Public status API (unauthenticated, for external dashboards)
	app.Get("/api/status", handlers.PublicStatus(database))

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
	app.Post("/login", loginLimiter, handlers.LoginPost(database, cfg, lockout))
	app.Get("/logout", handlers.Logout(cfg, database))

	// Protected routes
	protected := app.Group("/", auth.AuthMiddleware(cfg.JWTSecret, database))

	// General rate limiter for protected routes
	protected.Use(limiter.New(limiter.Config{
		Max:        60,
		Expiration: 1 * time.Minute,
		KeyGenerator: func(c *fiber.Ctx) string {
			return c.IP()
		},
	}))

	// CSRF protection
	//
	// CookieHTTPOnly MUST remain false: the HTMX configRequest handler in
	// base.templ reads the csrf_token cookie via document.cookie and copies
	// it into the X-CSRF-Token request header (double-submit pattern).
	protected.Use(csrf.New(csrf.Config{
		KeyLookup:      "header:X-CSRF-Token",
		CookieName:     "csrf_token",
		CookieSameSite: "Lax",
		CookieHTTPOnly: false,
		CookieSecure:   cfg.SecureCookies,
		Expiration:     1 * time.Hour,
	}))

	// Dashboard
	protected.Get("/dashboard", handlers.Dashboard(database))

	// Read-only routes (any authenticated user)
	protected.Get("/customers", handlers.ListCustomers(database))
	protected.Get("/customers/:id/edit", handlers.EditCustomerForm(database))
	protected.Get("/customers/:id/cancel", handlers.CancelEditCustomer(database))
	protected.Get("/servers", handlers.ListServers(database))
	protected.Get("/servers/:id/edit", handlers.EditServerForm(database))
	protected.Get("/servers/:id/row", handlers.CancelEditServer(database))
	protected.Get("/sites", handlers.ListSites(database))
	protected.Get("/sites/new", handlers.CreateSiteForm(database))
	protected.Get("/sites/:id", handlers.SiteDetail(database))
	protected.Get("/sites/:id/deploy/stream", handlers.DeploySSE(database))
	protected.Get("/sites/:id/logs", handlers.GetSiteLogs(database))
	protected.Get("/sites/:id/health", handlers.GetSiteHealth(database))
	protected.Get("/sites/:id/env", handlers.ListSiteEnvVars(database))
	protected.Get("/import", handlers.ImportPage())
	protected.Get("/payments", handlers.ListPayments(database))
	protected.Get("/payments/:id/edit", handlers.EditPaymentForm(database))
	protected.Get("/payments/:id/row", handlers.CancelEditPayment(database))
	protected.Get("/export/sites", handlers.ExportSitesCSV(database))
	protected.Get("/export/customers", handlers.ExportCustomersCSV(database))
	protected.Get("/export/payments", handlers.ExportPaymentsCSV(database))
	protected.Get("/backups", handlers.BackupsPage(backupMgr))
	protected.Get("/backups/:name/download", handlers.DownloadBackup(backupMgr))
	protected.Get("/api/templates", handlers.ListTemplates(database))

	// Write routes (admin only via WriteProtect)
	write := protected.Group("/", auth.WriteProtect())

	// Customer writes
	write.Post("/customers", handlers.CreateCustomer(database))
	write.Put("/customers/:id", handlers.UpdateCustomer(database))
	write.Delete("/customers/:id", handlers.DeleteCustomer(database))

	// Server writes
	write.Post("/servers", handlers.CreateServerHandler(database, cfg.SSHKeyDir))
	write.Put("/servers/:id", handlers.UpdateServerHandler(database, cfg.SSHKeyDir))
	write.Delete("/servers/:id", handlers.DeleteServerHandler(database))
	write.Post("/servers/:id/test", handlers.TestServerConnection(database))

	// Site writes
	write.Post("/sites/bulk", handlers.BulkSiteAction(database))
	write.Post("/sites", handlers.CreateSite(database, caddyMgr))
	write.Put("/sites/:id", handlers.UpdateSite(database, caddyMgr))
	write.Delete("/sites/:id", handlers.DeleteSite(database, caddyMgr))
	write.Post("/sites/:id/deploy", handlers.DeploySite(database))
	write.Post("/sites/:id/start", handlers.StartSite(database))
	write.Post("/sites/:id/stop", handlers.StopSite(database))
	write.Post("/sites/:id/restart", handlers.RestartSite(database))

	// Site env var writes
	write.Post("/sites/:id/env", handlers.CreateSiteEnvVar(database))
	write.Delete("/sites/:id/env/:varId", handlers.DeleteSiteEnvVar(database))

	// Import writes
	write.Post("/import/scan", handlers.ScanProjects(database))
	write.Post("/import", handlers.ImportProject(database, caddyMgr))

	// Payment writes
	write.Post("/payments", handlers.CreatePayment(database))
	write.Put("/payments/:id", handlers.UpdatePayment(database))
	write.Post("/payments/:id/mark-paid", handlers.MarkPaid(database))
	write.Delete("/payments/:id", handlers.DeletePayment(database))

	// Backup writes (admin only)
	write.Post("/backups/database", handlers.CreateDatabaseBackup(backupMgr, cfg.DBPath))
	write.Post("/backups/full", handlers.CreateFullBackup(backupMgr, cfg.DBPath))
	write.Post("/sites/:id/backup", handlers.CreateSiteBackupHandler(backupMgr, func(id int) (*models.Site, error) {
		return models.GetSiteByID(database, id)
	}))
	write.Delete("/backups/:name", handlers.DeleteBackup(backupMgr))
	write.Post("/backups/:name/restore", handlers.RestoreBackup(backupMgr, cfg.DBPath))

	// User management (admin only — extra AdminOnly guard)
	adminOnly := protected.Group("/", auth.AdminOnly())
	adminOnly.Get("/users", handlers.ListUsers(database))
	adminOnly.Post("/users", handlers.CreateUser(database))
	adminOnly.Delete("/users/:id", handlers.DeleteUserHandler(database))

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

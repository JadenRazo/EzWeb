package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           string
	JWTSecret      string
	AdminUser      string
	AdminPass      string
	DBPath         string
	CaddyfilePath  string
	AcmeEmail      string
	SecureCookies  bool
	WebhookURL     string
	WebhookFormat  string
	AlertThreshold int
	BackupDir      string
	MetricsEnabled        bool
	HealthCheckInterval   int
	JWTExpiryHours        int
	DBMaxOpenConns        int
	DBMaxIdleConns        int
	ActivityRetentionDays int
	HealthRetentionDays   int
	LockoutMaxAttempts    int
	LockoutDurationMin    int
	SMTPHost     string
	SMTPPort     int
	SMTPFrom     string
	SMTPUsername string
	SMTPPassword string
	AlertEmail   string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Port:           getEnv("APP_PORT", "3000"),
		JWTSecret:      getEnv("JWT_SECRET", ""),
		AdminUser:      getEnv("ADMIN_USER", "admin"),
		AdminPass:      getEnv("ADMIN_PASS", ""),
		DBPath:         getEnv("DB_PATH", "./ezweb.db"),
		CaddyfilePath:  getEnv("CADDYFILE_PATH", "/etc/caddy/Caddyfile"),
		AcmeEmail:      getEnv("ACME_EMAIL", ""),
		SecureCookies:  getEnv("SECURE_COOKIES", "false") == "true",
		WebhookURL:     getEnv("WEBHOOK_URL", ""),
		WebhookFormat:  getEnv("WEBHOOK_FORMAT", "discord"),
		AlertThreshold: getEnvInt("ALERT_THRESHOLD", 3),
		BackupDir:      getEnv("BACKUP_DIR", "./backups"),
		MetricsEnabled:        getEnv("METRICS_ENABLED", "false") == "true",
		HealthCheckInterval:   getEnvInt("HEALTH_CHECK_INTERVAL", 5),
		JWTExpiryHours:        getEnvInt("JWT_EXPIRY_HOURS", 24),
		DBMaxOpenConns:        getEnvInt("DB_MAX_OPEN_CONNS", 25),
		DBMaxIdleConns:        getEnvInt("DB_MAX_IDLE_CONNS", 5),
		ActivityRetentionDays: getEnvInt("ACTIVITY_RETENTION_DAYS", 90),
		HealthRetentionDays:   getEnvInt("HEALTH_RETENTION_DAYS", 30),
		LockoutMaxAttempts:    getEnvInt("LOCKOUT_MAX_ATTEMPTS", 5),
		LockoutDurationMin:    getEnvInt("LOCKOUT_DURATION_MIN", 15),
		SMTPHost:     getEnv("SMTP_HOST", ""),
		SMTPPort:     getEnvInt("SMTP_PORT", 587),
		SMTPFrom:     getEnv("SMTP_FROM", ""),
		SMTPUsername: getEnv("SMTP_USERNAME", ""),
		SMTPPassword: getEnv("SMTP_PASSWORD", ""),
		AlertEmail:   getEnv("ALERT_EMAIL", ""),
	}

	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	if cfg.AdminPass == "" {
		return nil, fmt.Errorf("ADMIN_PASS is required")
	}
	if len(cfg.AdminPass) < 8 {
		log.Println("WARNING: ADMIN_PASS is shorter than 8 characters — use a stronger password in production")
	}

	if len(cfg.JWTSecret) < 32 {
		log.Println("WARNING: JWT_SECRET is shorter than 32 characters — use a longer secret in production")
	}

	if cfg.BackupDir != "" {
		if err := os.MkdirAll(cfg.BackupDir, 0750); err != nil {
			log.Printf("WARNING: could not create BACKUP_DIR %q: %v", cfg.BackupDir, err)
		}
	}

	caddyParent := filepath.Dir(cfg.CaddyfilePath)
	if _, err := os.Stat(caddyParent); os.IsNotExist(err) {
		log.Printf("WARNING: CADDYFILE_PATH parent directory %q does not exist — Caddy config writes will fail", caddyParent)
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	if val, ok := os.LookupEnv(key); ok {
		return val
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if val, ok := os.LookupEnv(key); ok {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return fallback
}

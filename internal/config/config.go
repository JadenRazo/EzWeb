package config

import (
	"fmt"
	"log"
	"os"
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

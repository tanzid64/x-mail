// Package config loads and validates process configuration from the
// environment. Every value the application needs is resolved once at startup so
// that a misconfigured deployment fails immediately rather than on first use.
package config

import (
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	HTTPAddr    string
	CORSOrigins []string
	AppBaseURL  string

	DatabaseURL string

	RedisAddr     string
	RedisPassword string
	RedisDB       int

	SessionSecret       []byte
	SessionCookieName   string
	SessionTTL          time.Duration
	SessionCookieSecure bool

	EncryptionMasterKey []byte

	StorageDriver    string
	StorageLocalPath string
	S3Endpoint       string
	S3Region         string
	S3Bucket         string
	S3AccessKeyID    string
	S3SecretKey      string

	SystemResendAPIKey string
	SystemMailFrom     string

	ResendWebhookSecret string

	// ResendBaseURL overrides the Resend API endpoint. It exists so end-to-end
	// tests can point the stack at a stub provider, and is empty in production.
	ResendBaseURL string
}

func (c Config) IsProduction() bool { return c.Env == "production" }

// Load reads .env files (best effort, real environment always wins) and then
// builds a validated Config. Missing or malformed required values are collected
// so the operator sees every problem at once instead of one per restart.
func Load() (*Config, error) {
	loadDotEnv()

	var problems []string
	req := func(key string) string {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			problems = append(problems, key+" is required")
		}
		return v
	}

	cfg := &Config{
		Env:         def("APP_ENV", "development"),
		HTTPAddr:    def("HTTP_ADDR", ":8090"),
		CORSOrigins: splitList(def("CORS_ORIGINS", "http://localhost:5180")),
		AppBaseURL:  def("APP_BASE_URL", "http://localhost:5180"),

		DatabaseURL: req("DATABASE_URL"),

		RedisAddr:     def("REDIS_ADDR", "localhost:6380"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),
		RedisDB:       defInt("REDIS_DB", 0),

		SessionCookieName:   def("SESSION_COOKIE_NAME", "xmail_session"),
		SessionTTL:          time.Duration(defInt("SESSION_TTL_HOURS", 720)) * time.Hour,
		SessionCookieSecure: defBool("SESSION_COOKIE_SECURE", false),

		StorageDriver:    def("STORAGE_DRIVER", "local"),
		StorageLocalPath: def("STORAGE_LOCAL_PATH", "./storage"),
		S3Endpoint:       os.Getenv("S3_ENDPOINT"),
		S3Region:         def("S3_REGION", "auto"),
		S3Bucket:         os.Getenv("S3_BUCKET"),
		S3AccessKeyID:    os.Getenv("S3_ACCESS_KEY_ID"),
		S3SecretKey:      os.Getenv("S3_SECRET_ACCESS_KEY"),

		SystemResendAPIKey: os.Getenv("SYSTEM_RESEND_API_KEY"),
		SystemMailFrom:     def("SYSTEM_MAIL_FROM", "x-mail <noreply@example.com>"),

		ResendWebhookSecret: os.Getenv("RESEND_WEBHOOK_SECRET"),
		ResendBaseURL:       os.Getenv("RESEND_BASE_URL"),
	}

	// Session secret signs session cookies; anything shorter than 32 bytes is
	// not worth the false sense of security.
	if secret := req("SESSION_SECRET"); secret != "" {
		raw := decodeKey(secret)
		if len(raw) < 32 {
			problems = append(problems, "SESSION_SECRET must decode to at least 32 bytes")
		}
		cfg.SessionSecret = raw
	}

	// Provider API keys are sealed with AES-256-GCM, which requires exactly 32 bytes.
	if key := req("ENCRYPTION_MASTER_KEY"); key != "" {
		raw := decodeKey(key)
		if len(raw) != 32 {
			problems = append(problems, fmt.Sprintf("ENCRYPTION_MASTER_KEY must decode to exactly 32 bytes, got %d", len(raw)))
		}
		cfg.EncryptionMasterKey = raw
	}

	switch cfg.StorageDriver {
	case "local":
	case "s3":
		for _, k := range []string{"S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY"} {
			if os.Getenv(k) == "" {
				problems = append(problems, k+" is required when STORAGE_DRIVER=s3")
			}
		}
	default:
		problems = append(problems, "STORAGE_DRIVER must be \"local\" or \"s3\"")
	}

	if cfg.IsProduction() && !cfg.SessionCookieSecure {
		problems = append(problems, "SESSION_COOKIE_SECURE must be true in production")
	}
	// Redirecting provider traffic in production would send real credentials to
	// whatever host was configured.
	if cfg.IsProduction() && cfg.ResendBaseURL != "" {
		problems = append(problems, "RESEND_BASE_URL must not be set in production")
	}

	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid configuration:\n  - %s", strings.Join(problems, "\n  - "))
	}
	return cfg, nil
}

// loadDotEnv looks for a .env beside the binary and in the two parent
// directories, so `go run ./cmd/server` from apps/api picks up the repo-root file.
func loadDotEnv() {
	for _, p := range []string{".env", "../.env", "../../.env"} {
		if _, err := os.Stat(p); err == nil {
			_ = godotenv.Load(p)
		}
	}
}

// decodeKey accepts base64 (standard or raw) and falls back to the literal
// bytes, so operators can paste either `openssl rand -base64 32` output or a
// plain passphrase.
func decodeKey(v string) []byte {
	if b, err := base64.StdEncoding.DecodeString(v); err == nil {
		return b
	}
	if b, err := base64.RawStdEncoding.DecodeString(v); err == nil {
		return b
	}
	return []byte(v)
}

func def(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func defInt(key string, fallback int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return fallback
}

func defBool(key string, fallback bool) bool {
	if v, err := strconv.ParseBool(strings.TrimSpace(os.Getenv(key))); err == nil {
		return v
	}
	return fallback
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

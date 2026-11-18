// Package config loads and validates BiteOrbit API configuration from the
// environment. It fails fast on invalid combinations (e.g. production with a
// mock payment adapter) so misconfiguration can never ship silently.
package config

import (
	"fmt"
	"os"
	"strings"
)

type Env string

const (
	EnvDevelopment Env = "development"
	EnvTest        Env = "test"
	EnvProduction  Env = "production"
)

type Config struct {
	Env                Env
	HTTPAddr           string
	DatabaseURL        string
	RedisURL           string
	SessionSecret      string
	CookieDomain       string
	AllowedOrigins     []string
	UploadDir          string
	PaymentAdapter     string
	SMSAdapter         string
	EmailAdapter       string
	SMTPAddr           string
	MapsAdapter        string
	SessionsCookieName string
}

// FromEnv builds a Config from process environment variables with BITEORBIT_ prefixes.
func FromEnv() (*Config, error) {
	c := &Config{
		Env:                Env(getenv("BITEORBIT_ENV", "development")),
		HTTPAddr:           getenv("BITEORBIT_HTTP_ADDR", "127.0.0.1:8080"),
		DatabaseURL:        getenv("BITEORBIT_DATABASE_URL", ""),
		RedisURL:           getenv("BITEORBIT_REDIS_URL", ""),
		SessionSecret:      getenv("BITEORBIT_SESSION_SECRET", ""),
		CookieDomain:       getenv("BITEORBIT_COOKIE_DOMAIN", ""),
		UploadDir:          getenv("BITEORBIT_UPLOAD_DIR", "./data/uploads"),
		PaymentAdapter:     strings.ToLower(getenv("BITEORBIT_PAYMENT_ADAPTER", "mock")),
		SMSAdapter:         strings.ToLower(getenv("BITEORBIT_SMS_ADAPTER", "fake")),
		EmailAdapter:       strings.ToLower(getenv("BITEORBIT_EMAIL_ADAPTER", "mailpit")),
		SMTPAddr:           getenv("BITEORBIT_SMTP_ADDR", "127.0.0.1:51025"),
		MapsAdapter:        strings.ToLower(getenv("BITEORBIT_MAPS_ADAPTER", "demo")),
		SessionsCookieName: "bo_session",
	}

	if origins := getenv("BITEORBIT_ALLOWED_ORIGINS", ""); origins != "" {
		for _, o := range strings.Split(origins, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				c.AllowedOrigins = append(c.AllowedOrigins, trimmed)
			}
		}
	}

	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate enforces safety invariants, especially production guards.
func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("BITEORBIT_DATABASE_URL is required")
	}
	if c.Env == EnvDevelopment {
		// Convenience default for local single-host development.
		if len(c.AllowedOrigins) == 0 {
			c.AllowedOrigins = []string{
				"http://localhost:3000", "http://localhost:3001",
				"http://localhost:3002", "http://localhost:3003",
				"http://127.0.0.1:3000", "http://127.0.0.1:3001",
				"http://127.0.0.1:3002", "http://127.0.0.1:3003",
			}
		}
	}
	switch c.Env {
	case EnvDevelopment, EnvTest, EnvProduction:
	default:
		return fmt.Errorf("BITEORBIT_ENV must be development|test|production, got %q", c.Env)
	}
	if c.Env == EnvProduction {
		if c.SessionSecret == "" || len(c.SessionSecret) < 32 {
			return fmt.Errorf("BITEORBIT_SESSION_SECRET must be set to >=32 bytes in production")
		}
		if c.PaymentAdapter == "mock" {
			return fmt.Errorf("mock payment adapter is forbidden in production; configure a real provider")
		}
		if c.SMSAdapter == "fake" {
			return fmt.Errorf("fake SMS adapter is forbidden in production")
		}
		if c.EmailAdapter == "console" || c.EmailAdapter == "mailpit" {
			return fmt.Errorf("%s email adapter is forbidden in production", c.EmailAdapter)
		}
		if c.MapsAdapter == "demo" {
			return fmt.Errorf("demo maps adapter is forbidden in production")
		}
	}
	if c.Env == EnvDevelopment && c.SessionSecret == "" {
		// Deterministic local secret so restarts don't invalidate dev sessions.
		c.SessionSecret = "local-development-secret-do-not-use-xx"
	}
	if c.Env == EnvTest && c.SessionSecret == "" {
		c.SessionSecret = "test-secret-0123456789abcdef0123456789abcdef"
	}
	return nil
}

// DevLike reports whether simulated providers are allowed.
func (c *Config) DevLike() bool {
	return c.Env == EnvDevelopment || c.Env == EnvTest
}

func getenv(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

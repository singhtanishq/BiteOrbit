package config

import (
	"strings"
	"testing"
)

func TestValidateProductionGuards(t *testing.T) {
	base := func() *Config {
		return &Config{
			Env:            EnvProduction,
			DatabaseURL:    "postgres://x",
			SessionSecret:  strings.Repeat("s", 32),
			PaymentAdapter: "stripe",
			SMSAdapter:     "twilio",
			EmailAdapter:   "ses",
			MapsAdapter:    "mapbox",
		}
	}
	if err := base().Validate(); err != nil {
		t.Fatalf("valid production config rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{"short secret", func(c *Config) { c.SessionSecret = "too-short" }},
		{"mock payment", func(c *Config) { c.PaymentAdapter = "mock" }},
		{"fake sms", func(c *Config) { c.SMSAdapter = "fake" }},
		{"mailpit email", func(c *Config) { c.EmailAdapter = "mailpit" }},
		{"demo maps", func(c *Config) { c.MapsAdapter = "demo" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("production config with %s must be rejected", tt.name)
			}
		})
	}
}

func TestValidateDevelopmentDefaults(t *testing.T) {
	c := &Config{Env: EnvDevelopment, DatabaseURL: "postgres://x"}
	if err := c.Validate(); err != nil {
		t.Fatalf("development config rejected: %v", err)
	}
	if c.SessionSecret == "" {
		t.Fatal("development should get a deterministic fallback secret")
	}
	if len(c.AllowedOrigins) == 0 {
		t.Fatal("development should default to local app origins")
	}
}

func TestValidateMissingDatabase(t *testing.T) {
	c := &Config{Env: EnvDevelopment}
	if err := c.Validate(); err == nil {
		t.Fatal("missing database URL must be rejected")
	}
}

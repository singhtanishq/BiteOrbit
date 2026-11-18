// Package itest provides the shared integration-test environment: a real
// PostgreSQL test database, applied migrations, and a live httptest server
// wired exactly like production.
package itest

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver

	"biteorbit/services/api/internal/auth"
	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/migrations"
	"biteorbit/services/api/internal/platform/notify"
	"biteorbit/services/api/internal/seed"
	"biteorbit/services/api/internal/server"
)

// Env is one integration test environment (shared per test package).
type Env struct {
	Pool   *pgxpool.Pool
	URL    string // base URL of the test server
	Email  *EmailRecorder
	server *httptest.Server
}

// EmailRecorder captures outbound emails so tests can assert on delivery and
// extract tokens without a real SMTP server.
type EmailRecorder struct {
	Messages []EmailMessage
}

// EmailMessage is one captured email.
type EmailMessage struct {
	To      string
	Subject string
	Body    string
}

func (r *EmailRecorder) Send(_ context.Context, to, subject, textBody string) error {
	r.Messages = append(r.Messages, EmailMessage{To: to, Subject: subject, Body: textBody})
	return nil
}

func (r *EmailRecorder) Name() string { return "test-recorder" }

// MainStart prepares the environment for a test package. Call from TestMain;
// defer env.Teardown(). Exits the process when infrastructure is unavailable.
func MainStart() *Env {
	adminURL := os.Getenv("BITEORBIT_DATABASE_URL")
	if adminURL == "" {
		adminURL = "postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit?sslmode=disable"
	}
	testURL := os.Getenv("BITEORBIT_TEST_DATABASE_URL")
	if testURL == "" {
		testURL = "postgres://biteorbit:biteorbit@127.0.0.1:54329/biteorbit_test?sslmode=disable"
	}

	admin, err := sql.Open("pgx", adminURL)
	if err != nil {
		slog.Error("itest: open admin db", slog.Any("err", err))
		os.Exit(1)
	}
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS biteorbit_test WITH (FORCE)`); err != nil {
		slog.Error("itest: drop test db", slog.Any("err", err))
		os.Exit(1)
	}
	if _, err := admin.Exec(`CREATE DATABASE biteorbit_test`); err != nil {
		slog.Error("itest: create test db", slog.Any("err", err))
		os.Exit(1)
	}
	admin.Close()

	sqldb, err := sql.Open("pgx", testURL)
	if err != nil {
		os.Exit(1)
	}
	if _, err := (&migrations.Runner{DB: sqldb}).Up(context.Background()); err != nil {
		slog.Error("itest: migrate test db", slog.Any("err", err))
		os.Exit(1)
	}
	sqldb.Close()

	pool, err := db.Connect(context.Background(), testURL)
	if err != nil {
		slog.Error("itest: connect test db", slog.Any("err", err))
		os.Exit(1)
	}

	cfg := &config.Config{
		Env:            config.EnvTest,
		DatabaseURL:    testURL,
		SessionSecret:  "test-secret-0123456789abcdef0123456789abcdef",
		AllowedOrigins: []string{"http://localhost:3000"},
		PaymentAdapter: "mock",
		SMSAdapter:     "fake",
		EmailAdapter:   "console",
		MapsAdapter:    "demo",
	}
	recorder := &EmailRecorder{}

	// Deps are assembled directly (not via BuildDeps) so the email recorder
	// is wired into every consumer, including the auth service.
	deps := server.Deps{
		Cfg:                   cfg,
		Pool:                  pool,
		Log:                   slog.Default(),
		Auth:                  auth.NewService(pool, "", false, recorder),
		Limit:                 httpx.NewMemoryRateLimiter(),
		Email:                 recorder,
		SMS:                   &notify.FakeSMS{Log: slog.Default()},
		LoginRateLimit:        1000,
		ResetRateLimit:        1000,
		ResetConfirmRateLimit: 1000,
		RegisterRateLimit:     1000,
		PlaceRateLimit:        1000,
	}

	handler, _, err := server.New(deps)
	if err != nil {
		slog.Error("itest: server assembly", slog.Any("err", err))
		os.Exit(1)
	}

	ts := httptest.NewServer(handler)
	env := &Env{Pool: pool, URL: ts.URL, Email: recorder, server: ts}
	return env
}

// Teardown releases the server and pool.
func (e *Env) Teardown() {
	e.server.Close()
	e.Pool.Close()
}

// SeedDemo loads a reduced synthetic dataset into the test database.
func (e *Env) SeedDemo(t *testing.T) {
	t.Helper()
	if err := e.MustSeed(); err != nil {
		t.Fatalf("seed test db: %v", err)
	}
}

// MustSeed is the testing.T-free variant used from TestMain.
func (e *Env) MustSeed() error {
	return seed.Run(context.Background(), e.Pool, slog.Default(), seed.SeedOptions{Orders: 40})
}

// NewJar creates a cookie jar for use with Do.
func (e *Env) NewJar() *cookiejar.Jar {
	jar, _ := cookiejar.New(nil)
	return jar
}

// Do performs a request against the test server with an optional cookie jar
// and returns (status, decoded envelope).
func (e *Env) Do(t *testing.T, jar *cookiejar.Jar, method, path string, body any) (int, Envelope) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer res.Body.Close()

	var env Envelope
	_ = json.NewDecoder(res.Body).Decode(&env)
	return res.StatusCode, env
}

// DoWithHeader performs a request with extra headers (e.g. Idempotency-Key).
func (e *Env) DoWithHeader(t *testing.T, jar *cookiejar.Jar, method, path string, body any, headers map[string]string) (int, Envelope) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, e.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{Timeout: 20 * time.Second, Jar: jar}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s %s: %v", method, path, err)
	}
	defer res.Body.Close()
	var env Envelope
	_ = json.NewDecoder(res.Body).Decode(&env)
	return res.StatusCode, env
}

// Envelope mirrors the API response shape for tests.
type Envelope struct {
	Data  json.RawMessage `json:"data"`
	Meta  map[string]any  `json:"meta"`
	Error *struct {
		Code    string         `json:"code"`
		Message string         `json:"message"`
		Details map[string]any `json:"details"`
	} `json:"error"`
}

// RegisterCustomer registers a fresh customer via the API and returns the id.
func (e *Env) RegisterCustomer(t *testing.T, email string) string {
	t.Helper()
	return e.RegisterCustomerWithJar(t, e.NewJar(), email)
}

// RegisterCustomerWithJar registers a customer keeping the session cookie in
// the provided jar (registration starts a session).
func (e *Env) RegisterCustomerWithJar(t *testing.T, jar *cookiejar.Jar, email string) string {
	t.Helper()
	status, env := e.Do(t, jar, "POST", "/api/v1/auth/register", map[string]any{
		"email": email, "password": "Passw0rdLocal!", "name": "Test Customer",
	})
	if status != http.StatusCreated {
		t.Fatalf("register: status %d", status)
	}
	var data struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return data.ID
}

package server_test

import (
	"log/slog"
	"net/http/httptest"
	"testing"

	"biteorbit/services/api/internal/auth"
	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/itest"
	"biteorbit/services/api/internal/platform/notify"
	"biteorbit/services/api/internal/server"
)

// TestLoginRateLimited proves the login limiter actually engages (production
// default 10/min/IP) rather than being decorative.
func TestLoginRateLimited(t *testing.T) {
	cfg := &config.Config{
		Env:            config.EnvTest,
		AllowedOrigins: []string{"http://localhost:3000"},
	}
	deps := server.Deps{
		Cfg:            cfg,
		Pool:           env.Pool,
		Log:            slog.Default(),
		Auth:           auth.NewService(env.Pool, "", false, env.Email),
		Limit:          httpx.NewMemoryRateLimiter(),
		Email:          env.Email,
		SMS:            &notify.FakeSMS{Log: slog.Default()},
		LoginRateLimit: 3,
	}
	handler, _, err := server.New(deps)
	if err != nil {
		t.Fatalf("server: %v", err)
	}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	limited := &limitedEnv{ts: ts}
	body := map[string]any{"email": "ratelimit@example.test", "password": "WrongPass123"}

	statuses := []int{}
	for i := 0; i < 5; i++ {
		statuses = append(statuses, limited.post(t, "/api/v1/auth/login", body))
	}
	if statuses[0] != 401 || statuses[1] != 401 || statuses[2] != 401 {
		t.Fatalf("first three attempts should be 401 (wrong password), got %v", statuses)
	}
	if statuses[3] != 429 || statuses[4] != 429 {
		t.Fatalf("subsequent attempts must be 429, got %v", statuses)
	}
}

// limitedEnv posts JSON to a standalone test server without the shared env jar.
type limitedEnv struct {
	ts *httptest.Server
}

func (l *limitedEnv) post(t *testing.T, path string, body map[string]any) int {
	t.Helper()
	env2 := &itest.Env{URL: l.ts.URL, Email: env.Email}
	status, _ := env2.Do(t, env2.NewJar(), "POST", path, body)
	return status
}

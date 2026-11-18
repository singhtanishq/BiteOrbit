package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"biteorbit/services/api/internal/auth"
	"biteorbit/services/api/internal/config"
	"biteorbit/services/api/internal/httpx"
)

// testRegistry builds the route registry without touching a database.
// Registration must not require connectivity; only handlers do.
func testRegistry(t *testing.T) (*httpx.Registry, *config.Config) {
	t.Helper()
	cfg := &config.Config{Env: config.EnvTest, AllowedOrigins: []string{"http://localhost:3000"}}
	deps := Deps{
		Cfg:   cfg,
		Auth:  auth.NewService(nil, "", false, nil),
		Limit: httpx.NewMemoryRateLimiter(),
	}
	_, rg, err := New(deps)
	if err != nil {
		t.Fatalf("build registry: %v", err)
	}
	return rg, cfg
}

// TestOpenAPIMatchesCommittedDoc ensures docs/api/openapi.json always equals
// the document generated from the live route registry. Update it with:
//
//	REGENERATE=1 go test ./internal/server -run TestOpenAPIMatchesCommittedDoc
func TestOpenAPIMatchesCommittedDoc(t *testing.T) {
	rg, _ := testRegistry(t)
	doc, err := OpenAPIDocument(rg)
	if err != nil {
		t.Fatalf("generate OpenAPI: %v", err)
	}

	path := filepath.Join("..", "..", "..", "..", "docs", "api", "openapi.json")
	if os.Getenv("REGENERATE") == "1" {
		if err := os.WriteFile(path, append(doc, '\n'), 0o644); err != nil {
			t.Fatalf("write committed doc: %v", err)
		}
		t.Log("regenerated docs/api/openapi.json")
		return
	}

	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("docs/api/openapi.json missing — run: REGENERATE=1 go test ./internal/server -run TestOpenAPIMatchesCommittedDoc (%v)", err)
	}

	var want, got any
	if err := json.Unmarshal(doc, &want); err != nil {
		t.Fatalf("unmarshal generated: %v", err)
	}
	if err := json.Unmarshal(committed, &got); err != nil {
		t.Fatalf("unmarshal committed: %v", err)
	}
	if !jsonEqual(want, got) {
		t.Fatalf("docs/api/openapi.json is stale. Regenerate with:\n  REGENERATE=1 go test ./internal/server -run TestOpenAPIMatchesCommittedDoc")
	}
}

// jsonEqual compares two decoded JSON values for deep equality. Maps compare
// key-by-key so formatting differences don't matter.
func jsonEqual(a, b any) bool {
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for k, v := range av {
			bvv, ok := bv[k]
			if !ok || !jsonEqual(v, bvv) {
				return false
			}
		}
		return true
	case []any:
		bv, ok := b.([]any)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if !jsonEqual(av[i], bv[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}

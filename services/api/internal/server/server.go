// Package server assembles the HTTP API: middleware chain, route registry,
// health endpoints and domain route registration.
package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
)

// New builds the fully-wired http.Handler for the API.
func New(deps Deps) (http.Handler, *httpx.Registry, error) {
	rg := httpx.NewRegistry()
	registerSystemRoutes(rg, deps)
	registerDomainRoutes(rg, deps)

	// The OpenAPI document is generated from the completed registry.
	rg.Handle(http.MethodGet, "/api/v1/openapi.json", httpx.RouteMeta{
		Summary: "OpenAPI document (generated from registered routes)", Tag: "system", Auth: "public",
	}, func(w http.ResponseWriter, r *http.Request) {
		doc, err := OpenAPIDocument(rg)
		if err != nil {
			httpx.WriteError(w, r, apperr.New(apperr.CodeInternal, "Could not build OpenAPI document."))
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(doc)
	})

	handler := chain(rg.Handler(),
		httpx.Recover(deps.Log),
		httpx.Logging(deps.Log),
		httpx.SecurityHeaders,
		httpx.WithRequestID,
		httpx.OriginCheck(deps.Cfg.AllowedOrigins),
	)
	return handler, rg, nil
}

func chain(final http.Handler, mw ...func(http.Handler) http.Handler) http.Handler {
	h := final
	for i := len(mw) - 1; i >= 0; i-- {
		h = mw[i](h)
	}
	return h
}

// registerSystemRoutes adds health/readiness endpoints. No secrets exposed.
func registerSystemRoutes(rg *httpx.Registry, deps Deps) {
	rg.Handle(http.MethodGet, "/api/v1/healthz", httpx.RouteMeta{
		Summary: "Liveness probe", Tag: "system", Auth: "public",
	}, func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteData(w, r, http.StatusOK, map[string]any{
			"status": "ok", "env": string(deps.Cfg.Env), "time": time.Now().UTC(),
		})
	})

	rg.Handle(http.MethodGet, "/api/v1/readyz", httpx.RouteMeta{
		Summary: "Readiness probe (database connectivity)", Tag: "system", Auth: "public",
	}, func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		if err := db.Ping(ctx, deps.Pool); err != nil {
			httpx.Fail(w, r, http.StatusServiceUnavailable, "service_unavailable",
				"Database is not reachable.", nil)
			return
		}
		httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": "ready"})
	})
}

// OpenAPIDocument serialises the registered routes into an OpenAPI 3.0 doc.
func OpenAPIDocument(rg *httpx.Registry) ([]byte, error) {
	paths := map[string]any{}
	for _, rt := range rg.Routes() {
		p := rt.Path
		ops, ok := paths[p].(map[string]any)
		if !ok {
			ops = map[string]any{}
			paths[p] = ops
		}
		ops[methodJSON(rt.Method)] = map[string]any{
			"summary":  rt.Meta.Summary,
			"tags":     []string{rt.Meta.Tag},
			"security": authSecurity(rt.Meta.Auth),
			"responses": map[string]any{
				"200":       map[string]any{"description": "Success (envelope)"},
				"4xx / 5xx": map[string]any{"description": "Error envelope { error: { code, message } }"},
			},
		}
	}
	doc := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "BiteOrbit API",
			"version":     "1.0.0",
			"description": "Versioned REST API for the BiteOrbit platform. All responses use the envelope { data, meta?, error? }. Authenticated endpoints use the HttpOnly bo_session cookie via the same-origin proxy. Money values are integer paise (1 INR = 100 paise).",
		},
		"servers": []any{map[string]any{"url": "/api/v1", "description": "same-origin proxy path"}},
		"paths":   paths,
	}
	return json.MarshalIndent(doc, "", "  ")
}

func authSecurity(auth string) []any {
	if auth == "public" {
		return []any{}
	}
	if auth == "any" {
		return []any{map[string]any{"sessionCookie": []string{}}}
	}
	return []any{map[string]any{"sessionRole": auth}}
}

func methodJSON(m string) string {
	switch m {
	case http.MethodPost:
		return "post"
	case http.MethodPut:
		return "put"
	case http.MethodPatch:
		return "patch"
	case http.MethodDelete:
		return "delete"
	default:
		return "get"
	}
}

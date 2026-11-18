package httpx

import (
	"context"
	"net/http"
	"sort"
)

// Principal is the authenticated caller attached to the request context.
type Principal struct {
	ID    string   `json:"id"`
	Name  string   `json:"name"`
	Email string   `json:"email"`
	Roles []string `json:"roles"`
}

func (p *Principal) HasRole(role string) bool {
	for _, r := range p.Roles {
		if r == role {
			return true
		}
	}
	return false
}

func (p *Principal) HasAnyRole(roles ...string) bool {
	for _, want := range roles {
		if p.HasRole(want) {
			return true
		}
	}
	return false
}

// UserFrom returns the authenticated principal, or nil for anonymous requests.
func UserFrom(ctx context.Context) *Principal {
	if v, ok := ctx.Value(ctxUser).(*Principal); ok {
		return v
	}
	return nil
}

func withUser(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, ctxUser, p)
}

// ContextWithUser attaches an authenticated principal (used by auth services).
func ContextWithUser(ctx context.Context, p *Principal) context.Context {
	return withUser(ctx, p)
}

// RouteMeta documents a registered route and drives OpenAPI generation.
type RouteMeta struct {
	Summary string
	Tag     string
	// Auth: "public", "any" (any authenticated user) or a role list string.
	Auth string
}

// Registry is the single place routes are declared. It registers onto an
// *http.ServeMux and records metadata for the OpenAPI route-coverage test.
type Registry struct {
	mux    *http.ServeMux
	routes []registeredRoute
}

type registeredRoute struct {
	Method string
	Path   string
	Meta   RouteMeta
}

func NewRegistry() *Registry {
	return &Registry{mux: http.NewServeMux()}
}

// Handle registers a handler for method+pattern (Go 1.22 ServeMux patterns).
// panics on duplicate registration, matching ServeMux behaviour.
func (rg *Registry) Handle(method, pattern string, meta RouteMeta, h http.HandlerFunc) {
	rg.mux.HandleFunc(method+" "+pattern, h)
	rg.routes = append(rg.routes, registeredRoute{Method: method, Path: pattern, Meta: meta})
}

// Handler exposes the underlying mux.
func (rg *Registry) Handler() http.Handler { return rg.mux }

// Routes returns the registered routes sorted for deterministic output.
func (rg *Registry) Routes() []registeredRoute {
	out := append([]registeredRoute(nil), rg.routes...)
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

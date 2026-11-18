package server

import (
	"net/http"
	"time"

	"biteorbit/services/api/internal/cart"
	"biteorbit/services/api/internal/discovery"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/users"
)

// registerDomainRoutes wires every domain's routes into the registry.
// Domains are added milestone by milestone:
//
//	M03: auth, me (profile), addresses   ← current
//	M04: discovery, restaurants, menus
//	M05: carts
//	M06: checkout, coupons
//	M07: orders (customer), payments
//	M08: restaurant-portal
//	M09: courier
//	M10: admin, support, notifications
func registerDomainRoutes(rg *httpx.Registry, deps Deps) {
	registerAuthRoutes(rg, deps)
	registerMeRoutes(rg, deps)
	registerDiscoveryRoutes(rg, deps)
	registerCartRoutes(rg, deps)
}

func registerCartRoutes(rg *httpx.Registry, deps Deps) {
	c := cart.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth

	rg.Handle(http.MethodGet, "/api/v1/cart", httpx.RouteMeta{
		Summary: "Caller's active cart with server-side revalidation", Tag: "cart", Auth: "any",
	}, auth(c.Get))

	rg.Handle(http.MethodPost, "/api/v1/cart/items", httpx.RouteMeta{
		Summary: "Add a configured item (requires confirmation to replace another outlet's cart)", Tag: "cart", Auth: "any",
	}, auth(c.AddItem))

	rg.Handle(http.MethodPatch, "/api/v1/cart/items/{id}", httpx.RouteMeta{
		Summary: "Update quantity or instructions of an owned cart line", Tag: "cart", Auth: "any",
	}, auth(c.UpdateItem))

	rg.Handle(http.MethodDelete, "/api/v1/cart/items/{id}", httpx.RouteMeta{
		Summary: "Remove an owned cart line", Tag: "cart", Auth: "any",
	}, auth(c.DeleteItem))

	rg.Handle(http.MethodDelete, "/api/v1/cart", httpx.RouteMeta{
		Summary: "Clear the active cart", Tag: "cart", Auth: "any",
	}, auth(c.Clear))
}

func registerDiscoveryRoutes(rg *httpx.Registry, deps Deps) {
	d := discovery.NewHandlers(deps.Pool)
	pub := deps.Auth.OptionalAuth

	rg.Handle(http.MethodGet, "/api/v1/discovery/restaurants", httpx.RouteMeta{
		Summary: "List/search restaurants with filters, sorting, pagination", Tag: "discovery", Auth: "public",
	}, pub(d.ListRestaurants))

	rg.Handle(http.MethodGet, "/api/v1/discovery/suggestions", httpx.RouteMeta{
		Summary: "Search suggestions across restaurants, cuisines and dishes", Tag: "discovery", Auth: "public",
	}, pub(d.Suggestions))

	rg.Handle(http.MethodGet, "/api/v1/discovery/dishes", httpx.RouteMeta{
		Summary: "Search dishes across all active outlets", Tag: "discovery", Auth: "public",
	}, pub(d.SearchDishes))

	rg.Handle(http.MethodGet, "/api/v1/discovery/cuisines", httpx.RouteMeta{
		Summary: "Cuisine filter chips with outlet counts", Tag: "discovery", Auth: "public",
	}, pub(d.ListCuisines))

	rg.Handle(http.MethodGet, "/api/v1/discovery/collections", httpx.RouteMeta{
		Summary: "Curated collections with outlet counts", Tag: "discovery", Auth: "public",
	}, pub(d.ListCollections))

	rg.Handle(http.MethodPost, "/api/v1/discovery/serviceability", httpx.RouteMeta{
		Summary: "Check demo-city serviceability for a point or locality", Tag: "discovery", Auth: "public",
	}, pub(d.Serviceability))

	rg.Handle(http.MethodGet, "/api/v1/restaurants/{id}", httpx.RouteMeta{
		Summary: "Outlet detail with hours, rating and address", Tag: "restaurants", Auth: "public",
	}, pub(d.GetOutlet))

	rg.Handle(http.MethodGet, "/api/v1/restaurants/{id}/menu", httpx.RouteMeta{
		Summary: "Full menu with categories, items and modifier groups", Tag: "restaurants", Auth: "public",
	}, pub(d.GetOutletMenu))
}

func registerAuthRoutes(rg *httpx.Registry, deps Deps) {
	a := deps.Auth
	loginLimiter := deps.LoginRateLimit
	if loginLimiter <= 0 {
		loginLimiter = 10
	}
	registerLimiter := deps.RegisterRateLimit
	if registerLimiter <= 0 {
		registerLimiter = 10
	}
	resetLimiter := deps.ResetRateLimit
	if resetLimiter <= 0 {
		resetLimiter = 5
	}
	resetConfirmLimiter := deps.ResetConfirmRateLimit
	if resetConfirmLimiter <= 0 {
		resetConfirmLimiter = 10
	}

	rg.Handle(http.MethodPost, "/api/v1/auth/register", httpx.RouteMeta{
		Summary: "Create a customer account and start a session", Tag: "auth", Auth: "public",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, registerLimiter, time.Minute, a.Register))

	rg.Handle(http.MethodPost, "/api/v1/auth/login", httpx.RouteMeta{
		Summary: "Sign in (enumeration-safe, rate limited)", Tag: "auth", Auth: "public",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, loginLimiter, time.Minute, a.Login))

	rg.Handle(http.MethodGet, "/api/v1/auth/session", httpx.RouteMeta{
		Summary: "Current session principal", Tag: "auth", Auth: "any",
	}, a.RequireAuth(a.Session))

	rg.Handle(http.MethodPost, "/api/v1/auth/refresh", httpx.RouteMeta{
		Summary: "Rotate the session token", Tag: "auth", Auth: "any",
	}, a.RequireAuth(a.RefreshSession))

	rg.Handle(http.MethodPost, "/api/v1/auth/logout", httpx.RouteMeta{
		Summary: "Revoke the current session", Tag: "auth", Auth: "any",
	}, a.RequireAuth(a.Logout))

	rg.Handle(http.MethodPost, "/api/v1/auth/logout-all", httpx.RouteMeta{
		Summary: "Revoke every session for the caller", Tag: "auth", Auth: "any",
	}, a.RequireAuth(a.LogoutAll))

	rg.Handle(http.MethodPost, "/api/v1/auth/password-reset", httpx.RouteMeta{
		Summary: "Request a password reset email (enumeration-safe)", Tag: "auth", Auth: "public",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, resetLimiter, time.Minute, a.PasswordResetRequest))

	rg.Handle(http.MethodPost, "/api/v1/auth/password-reset/confirm", httpx.RouteMeta{
		Summary: "Consume a reset token and set a new password", Tag: "auth", Auth: "public",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, resetConfirmLimiter, time.Minute, a.PasswordResetConfirm))
}

func registerMeRoutes(rg *httpx.Registry, deps Deps) {
	u := users.NewHandlers(deps.Pool)

	rg.Handle(http.MethodGet, "/api/v1/me", httpx.RouteMeta{
		Summary: "Caller profile and preferences", Tag: "me", Auth: "any",
	}, deps.Auth.RequireAuth(u.Me))

	rg.Handle(http.MethodPatch, "/api/v1/me", httpx.RouteMeta{
		Summary: "Update profile fields", Tag: "me", Auth: "any",
	}, deps.Auth.RequireAuth(u.UpdateMe))

	rg.Handle(http.MethodGet, "/api/v1/me/addresses", httpx.RouteMeta{
		Summary: "List the caller's addresses", Tag: "addresses", Auth: "any",
	}, deps.Auth.RequireAuth(u.ListAddresses))

	rg.Handle(http.MethodPost, "/api/v1/me/addresses", httpx.RouteMeta{
		Summary: "Create an address", Tag: "addresses", Auth: "any",
	}, deps.Auth.RequireAuth(u.CreateAddress))

	rg.Handle(http.MethodPatch, "/api/v1/me/addresses/{id}", httpx.RouteMeta{
		Summary: "Update an owned address", Tag: "addresses", Auth: "any",
	}, deps.Auth.RequireAuth(u.UpdateAddress))

	rg.Handle(http.MethodDelete, "/api/v1/me/addresses/{id}", httpx.RouteMeta{
		Summary: "Soft-delete an owned address", Tag: "addresses", Auth: "any",
	}, deps.Auth.RequireAuth(u.DeleteAddress))

	rg.Handle(http.MethodPost, "/api/v1/me/addresses/{id}/default", httpx.RouteMeta{
		Summary: "Set an owned address as default", Tag: "addresses", Auth: "any",
	}, deps.Auth.RequireAuth(u.SetDefaultAddress))
}

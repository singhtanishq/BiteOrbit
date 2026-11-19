package server

import (
	"net/http"
	"time"

	"biteorbit/services/api/internal/cart"
	"biteorbit/services/api/internal/checkout"
	"biteorbit/services/api/internal/discovery"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/orders"
	"biteorbit/services/api/internal/payments"
	"biteorbit/services/api/internal/portal"
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
	registerCheckoutRoutes(rg, deps)
	registerOrderRoutes(rg, deps)
	registerPaymentRoutes(rg, deps)
	registerPortalRoutes(rg, deps)
}

func registerPortalRoutes(rg *httpx.Registry, deps Deps) {
	pt := portal.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireRole("restaurant_owner", "restaurant_manager", "restaurant_staff")

	rg.Handle(http.MethodGet, "/api/v1/restaurant-portal/outlets", httpx.RouteMeta{
		Summary: "Outlets accessible to this staff member", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.MyOutlets))

	rg.Handle(http.MethodGet, "/api/v1/restaurant-portal/orders", httpx.RouteMeta{
		Summary: "Outlet order queue with item summaries", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.ListOutletOrders))

	rg.Handle(http.MethodGet, "/api/v1/restaurant-portal/menu", httpx.RouteMeta{
		Summary: "Outlet menu for management (includes unavailable items)", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.OutletMenu))

	rg.Handle(http.MethodPatch, "/api/v1/restaurant-portal/menu/items/{id}", httpx.RouteMeta{
		Summary: "Update item name/description/price/availability (audited)", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.UpdateMenuItem))

	rg.Handle(http.MethodGet, "/api/v1/restaurant-portal/analytics", httpx.RouteMeta{
		Summary: "Outlet analytics summary with documented metric definitions", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.Analytics))

	rg.Handle(http.MethodPost, "/api/v1/restaurant-portal/orders/{id}/accept", httpx.RouteMeta{
		Summary: "Accept a placed order", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.Accept))

	rg.Handle(http.MethodPost, "/api/v1/restaurant-portal/orders/{id}/reject", httpx.RouteMeta{
		Summary: "Reject a placed order with a reason", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.Reject))

	rg.Handle(http.MethodPost, "/api/v1/restaurant-portal/orders/{id}/prepare", httpx.RouteMeta{
		Summary: "Move a confirmed order to preparing", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.StartPrepare))

	rg.Handle(http.MethodPost, "/api/v1/restaurant-portal/orders/{id}/ready", httpx.RouteMeta{
		Summary: "Mark a preparing order ready for pickup", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(pt.MarkReady))
}

func registerOrderRoutes(rg *httpx.Registry, deps Deps) {
	o := orders.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth

	rg.Handle(http.MethodGet, "/api/v1/orders", httpx.RouteMeta{
		Summary: "Caller's order history (paginated)", Tag: "orders", Auth: "any",
	}, auth(o.List))

	rg.Handle(http.MethodGet, "/api/v1/orders/{id}", httpx.RouteMeta{
		Summary: "Order detail with timeline (owner-only)", Tag: "orders", Auth: "any",
	}, auth(o.Get))

	rg.Handle(http.MethodPost, "/api/v1/orders/{id}/cancel", httpx.RouteMeta{
		Summary: "Cancel an order within the policy window (owner-only)", Tag: "orders", Auth: "any",
	}, auth(o.Cancel))
}

func registerPaymentRoutes(rg *httpx.Registry, deps Deps) {
	pm := payments.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth

	rg.Handle(http.MethodGet, "/api/v1/orders/{id}/payment", httpx.RouteMeta{
		Summary: "Latest payment for an owned order", Tag: "payments", Auth: "any",
	}, auth(pm.ByOrder))

	rg.Handle(http.MethodPost, "/api/v1/payments/{id}/simulate", httpx.RouteMeta{
		Summary: "Simulated gateway result (mock provider only; goes through the signed webhook path)", Tag: "payments", Auth: "any",
	}, auth(pm.Simulate))

	rg.Handle(http.MethodPost, "/api/v1/orders/{id}/refund", httpx.RouteMeta{
		Summary: "Request a refund for a captured online payment (owner-only)", Tag: "payments", Auth: "any",
	}, auth(pm.RequestRefund))

	rg.Handle(http.MethodPost, "/api/v1/webhooks/mock", httpx.RouteMeta{
		Summary: "Mock provider webhook (HMAC-verified, idempotent)", Tag: "webhooks", Auth: "public",
	}, pm.Webhook)
}

func registerCheckoutRoutes(rg *httpx.Registry, deps Deps) {
	c := checkout.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth
	placeLimiter := deps.PlaceRateLimit
	if placeLimiter <= 0 {
		placeLimiter = 20
	}

	rg.Handle(http.MethodPost, "/api/v1/checkout/quote", httpx.RouteMeta{
		Summary: "Compute the authoritative price quote for the current cart", Tag: "checkout", Auth: "any",
	}, auth(c.Quote))

	rg.Handle(http.MethodPost, "/api/v1/checkout/place", httpx.RouteMeta{
		Summary: "Place an order from a valid quote (Idempotency-Key required)", Tag: "checkout", Auth: "any",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, placeLimiter, time.Minute, auth(c.Place)))

	rg.Handle(http.MethodPost, "/api/v1/checkout/coupon", httpx.RouteMeta{
		Summary: "Validate a coupon against the current cart subtotal", Tag: "coupons", Auth: "any",
	}, auth(c.ValidateCoupon))
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

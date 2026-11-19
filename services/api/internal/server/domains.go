package server

import (
	"net/http"
	"time"

	"biteorbit/services/api/internal/admin"
	"biteorbit/services/api/internal/cart"
	"biteorbit/services/api/internal/checkout"
	"biteorbit/services/api/internal/courier"
	"biteorbit/services/api/internal/discovery"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/notifications"
	"biteorbit/services/api/internal/orders"
	"biteorbit/services/api/internal/payments"
	"biteorbit/services/api/internal/portal"
	"biteorbit/services/api/internal/reviews"
	"biteorbit/services/api/internal/support"
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
	registerCourierRoutes(rg, deps)
	registerAdminRoutes(rg, deps)
	registerSupportRoutes(rg, deps)
	registerNotificationRoutes(rg, deps)
	registerReviewRoutes(rg, deps)
}

func registerAdminRoutes(rg *httpx.Registry, deps Deps) {
	a := admin.NewHandlers(deps.Pool)
	adminOnly := deps.Auth.RequireRole("admin", "ops_manager", "support_agent")
	adminFull := deps.Auth.RequireRole("admin", "ops_manager")

	rg.Handle(http.MethodGet, "/api/v1/admin/overview", httpx.RouteMeta{
		Summary: "Platform metrics overview with documented definitions", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.Overview))

	rg.Handle(http.MethodGet, "/api/v1/admin/orders", httpx.RouteMeta{
		Summary: "Platform-wide order search", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListOrders))

	rg.Handle(http.MethodGet, "/api/v1/admin/orders/export.csv", httpx.RouteMeta{
		Summary: "Orders report CSV export (ops roles)", Tag: "admin", Auth: "admin roles",
	}, adminOnly(orders.AdminOrdersExport(deps.Pool)))

	rg.Handle(http.MethodPost, "/api/v1/admin/orders/{id}/cancel", httpx.RouteMeta{
		Summary: "Audited administrative cancellation (ops roles only)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.AdminCancel))

	rg.Handle(http.MethodGet, "/api/v1/admin/users", httpx.RouteMeta{
		Summary: "User list (data-minimised)", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListUsers))

	rg.Handle(http.MethodPost, "/api/v1/admin/users/{id}/suspend", httpx.RouteMeta{
		Summary: "Suspend a user (audited, sessions revoked)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.SuspendUser))

	rg.Handle(http.MethodPost, "/api/v1/admin/users/{id}/activate", httpx.RouteMeta{
		Summary: "Reactivate a user (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.ActivateUser))

	rg.Handle(http.MethodGet, "/api/v1/admin/restaurants", httpx.RouteMeta{
		Summary: "Organisations and outlets", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListRestaurants))

	rg.Handle(http.MethodGet, "/api/v1/admin/applications", httpx.RouteMeta{
		Summary: "Restaurant applications", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListApplications))

	rg.Handle(http.MethodPost, "/api/v1/admin/applications/{id}/review", httpx.RouteMeta{
		Summary: "Review an application (demo approval is a simulation)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.ReviewApplication))

	rg.Handle(http.MethodGet, "/api/v1/admin/riders", httpx.RouteMeta{
		Summary: "Delivery partners with status and load", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListRiders))

	rg.Handle(http.MethodPost, "/api/v1/admin/riders/{id}/approve", httpx.RouteMeta{
		Summary: "Approve a rider (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.ApproveRider))

	rg.Handle(http.MethodPost, "/api/v1/admin/riders/{id}/suspend", httpx.RouteMeta{
		Summary: "Suspend a rider (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.SuspendRider))

	rg.Handle(http.MethodGet, "/api/v1/admin/coupons", httpx.RouteMeta{
		Summary: "Coupon management list", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListCoupons))

	rg.Handle(http.MethodPost, "/api/v1/admin/coupons", httpx.RouteMeta{
		Summary: "Create a coupon (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.CreateCoupon))

	rg.Handle(http.MethodPatch, "/api/v1/admin/coupons/{id}", httpx.RouteMeta{
		Summary: "Activate/deactivate a coupon (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.UpdateCoupon))

	rg.Handle(http.MethodGet, "/api/v1/admin/audit-logs", httpx.RouteMeta{
		Summary: "Audit log trail", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListAuditLogs))

	rg.Handle(http.MethodGet, "/api/v1/admin/feature-flags", httpx.RouteMeta{
		Summary: "Feature flags", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.FeatureFlags))

	rg.Handle(http.MethodPatch, "/api/v1/admin/feature-flags/{key}", httpx.RouteMeta{
		Summary: "Toggle a feature flag (audited)", Tag: "admin", Auth: "admin roles",
	}, adminFull(a.UpdateFlag))

	rg.Handle(http.MethodGet, "/api/v1/admin/support/tickets", httpx.RouteMeta{
		Summary: "Support queue", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ListTickets))

	rg.Handle(http.MethodGet, "/api/v1/admin/support/tickets/{id}", httpx.RouteMeta{
		Summary: "Ticket thread (includes internal notes)", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.TicketDetail))

	rg.Handle(http.MethodPost, "/api/v1/admin/support/tickets/{id}/reply", httpx.RouteMeta{
		Summary: "Reply to a ticket (internal notes supported)", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.ReplyTicket))

	rg.Handle(http.MethodPost, "/api/v1/admin/support/tickets/{id}/status", httpx.RouteMeta{
		Summary: "Update ticket status/priority", Tag: "admin", Auth: "admin roles",
	}, adminOnly(a.UpdateTicket))
}

func registerSupportRoutes(rg *httpx.Registry, deps Deps) {
	s := support.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth
	ticketLimiter := deps.TicketRateLimit
	if ticketLimiter <= 0 {
		ticketLimiter = 10
	}

	rg.Handle(http.MethodPost, "/api/v1/support/tickets", httpx.RouteMeta{
		Summary: "Create a support ticket (optionally order-linked)", Tag: "support", Auth: "any",
	}, httpx.RateLimit(deps.Limit, httpx.KeyByIP, ticketLimiter, time.Minute, auth(s.CreateTicket)))

	rg.Handle(http.MethodGet, "/api/v1/support/tickets", httpx.RouteMeta{
		Summary: "Caller's own tickets", Tag: "support", Auth: "any",
	}, auth(s.ListMine))

	rg.Handle(http.MethodGet, "/api/v1/support/tickets/{id}", httpx.RouteMeta{
		Summary: "Owner-only thread (internal notes excluded)", Tag: "support", Auth: "any",
	}, auth(s.GetThread))

	rg.Handle(http.MethodPost, "/api/v1/support/tickets/{id}/messages", httpx.RouteMeta{
		Summary: "Reply on an owned ticket", Tag: "support", Auth: "any",
	}, auth(s.Reply))
}

func registerNotificationRoutes(rg *httpx.Registry, deps Deps) {
	n := notifications.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireAuth

	rg.Handle(http.MethodGet, "/api/v1/notifications", httpx.RouteMeta{
		Summary: "In-app notification centre", Tag: "notifications", Auth: "any",
	}, auth(n.List))

	rg.Handle(http.MethodPost, "/api/v1/notifications/{id}/read", httpx.RouteMeta{
		Summary: "Mark a notification read (owner-only)", Tag: "notifications", Auth: "any",
	}, auth(n.MarkRead))

	rg.Handle(http.MethodPost, "/api/v1/notifications/read-all", httpx.RouteMeta{
		Summary: "Mark all notifications read", Tag: "notifications", Auth: "any",
	}, auth(n.MarkAllRead))

	rg.Handle(http.MethodGet, "/api/v1/notifications/preferences", httpx.RouteMeta{
		Summary: "Per-category delivery preferences", Tag: "notifications", Auth: "any",
	}, auth(n.Preferences))

	rg.Handle(http.MethodPatch, "/api/v1/notifications/preferences", httpx.RouteMeta{
		Summary: "Update per-category delivery preferences", Tag: "notifications", Auth: "any",
	}, auth(n.UpdatePreference))
}

func registerCourierRoutes(rg *httpx.Registry, deps Deps) {
	c := courier.NewHandlers(deps.Pool)
	auth := deps.Auth.RequireRole("rider")

	rg.Handle(http.MethodGet, "/api/v1/courier/me", httpx.RouteMeta{
		Summary: "Delivery partner profile", Tag: "courier", Auth: "rider",
	}, auth(c.Profile))

	rg.Handle(http.MethodPost, "/api/v1/courier/availability", httpx.RouteMeta{
		Summary: "Go online/offline (approved riders only)", Tag: "courier", Auth: "rider",
	}, auth(c.SetAvailability))

	rg.Handle(http.MethodGet, "/api/v1/courier/offers", httpx.RouteMeta{
		Summary: "Open delivery offers (drop locality only until accepted)", Tag: "courier", Auth: "rider",
	}, auth(c.Offers))

	rg.Handle(http.MethodPost, "/api/v1/courier/offers/{id}/accept", httpx.RouteMeta{
		Summary: "Atomically claim a delivery offer", Tag: "courier", Auth: "rider",
	}, auth(c.AcceptOffer))

	rg.Handle(http.MethodGet, "/api/v1/courier/assignments/{id}", httpx.RouteMeta{
		Summary: "Assignment detail with pickup/drop info and allowed transitions", Tag: "courier", Auth: "rider",
	}, auth(c.Assignment))

	rg.Handle(http.MethodPost, "/api/v1/courier/assignments/{id}/state", httpx.RouteMeta{
		Summary: "Advance the delivery lifecycle (OTP-verified delivery)", Tag: "courier", Auth: "rider",
	}, auth(c.UpdateAssignmentState))

	rg.Handle(http.MethodGet, "/api/v1/courier/earnings", httpx.RouteMeta{
		Summary: "Earnings ledger summary (rider-editable amounts never)", Tag: "courier", Auth: "rider",
	}, auth(c.Earnings))
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

	rg.Handle(http.MethodGet, "/api/v1/restaurant-portal/analytics/export.csv", httpx.RouteMeta{
		Summary: "Outlet sales report CSV export", Tag: "restaurant-portal", Auth: "restaurant roles",
	}, auth(orders.RestaurantSalesExport(deps.Pool)))

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

	rg.Handle(http.MethodGet, "/api/v1/orders/{id}/track", httpx.RouteMeta{
		Summary: "Owner-only live tracking with deterministic demo position", Tag: "orders", Auth: "any",
	}, auth(o.Track))

	rg.Handle(http.MethodPost, "/api/v1/orders/{id}/reorder", httpx.RouteMeta{
		Summary: "Rebuild the cart from a past order at current prices", Tag: "orders", Auth: "any",
	}, auth(o.Reorder))

	rg.Handle(http.MethodGet, "/api/v1/me/favourites", httpx.RouteMeta{
		Summary: "Favourite outlets", Tag: "favourites", Auth: "any",
	}, auth(o.ListFavourites))

	rg.Handle(http.MethodPost, "/api/v1/me/favourites/{outlet_id}", httpx.RouteMeta{
		Summary: "Favourite an outlet", Tag: "favourites", Auth: "any",
	}, auth(o.AddFavourite))

	rg.Handle(http.MethodDelete, "/api/v1/me/favourites/{outlet_id}", httpx.RouteMeta{
		Summary: "Unfavourite an outlet", Tag: "favourites", Auth: "any",
	}, auth(o.RemoveFavourite))
}

func registerReviewRoutes(rg *httpx.Registry, deps Deps) {
	rv := reviews.NewHandlers(deps.Pool)

	rg.Handle(http.MethodPost, "/api/v1/reviews", httpx.RouteMeta{
		Summary: "Review a delivered order (once per order)", Tag: "reviews", Auth: "any",
	}, deps.Auth.RequireAuth(rv.Create))

	rg.Handle(http.MethodGet, "/api/v1/restaurants/{id}/reviews", httpx.RouteMeta{
		Summary: "Published reviews for an outlet", Tag: "reviews", Auth: "public",
	}, deps.Auth.OptionalAuth(rv.ListForOutlet))

	rg.Handle(http.MethodPost, "/api/v1/reviews/{id}/report", httpx.RouteMeta{
		Summary: "Report a review for moderation", Tag: "reviews", Auth: "any",
	}, deps.Auth.RequireAuth(rv.Report))

	rg.Handle(http.MethodGet, "/api/v1/me/reviews", httpx.RouteMeta{
		Summary: "Caller's reviews", Tag: "reviews", Auth: "any",
	}, deps.Auth.RequireAuth(rv.Mine))
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

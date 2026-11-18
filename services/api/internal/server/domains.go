package server

import "biteorbit/services/api/internal/httpx"

// registerDomainRoutes wires every domain's routes into the registry.
// Domains are added milestone by milestone:
//
//	M03: auth, me (profile), addresses
//	M04: discovery, restaurants, menus
//	M05: carts
//	M06: checkout, coupons
//	M07: orders (customer), payments
//	M08: restaurant-portal
//	M09: courier
//	M10: admin, support, notifications
func registerDomainRoutes(rg *httpx.Registry, deps Deps) {
	// Filled in from Milestone 03 onwards.
	_ = rg
	_ = deps
}

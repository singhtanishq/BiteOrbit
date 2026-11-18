package auth

import (
	"net/http"

	"biteorbit/services/api/internal/httpx"
)

// RequireAuth resolves the session cookie into a principal, rejecting
// anonymous requests with 401.
func (s *Service) RequireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := s.Lookup(r.Context(), r)
		if err != nil {
			httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized",
				"Please sign in to continue.", nil)
			return
		}
		next(w, r.WithContext(httpx.ContextWithUser(r.Context(), p)))
	}
}

// RequireRole enforces server-side RBAC. Role checks on the client are UX;
// this is the enforcement point.
func (s *Service) RequireRole(roles ...string) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			p, err := s.Lookup(r.Context(), r)
			if err != nil {
				httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized",
					"Please sign in to continue.", nil)
				return
			}
			if !p.HasAnyRole(roles...) {
				httpx.Fail(w, r, http.StatusForbidden, "forbidden",
					"You do not have permission to perform this action.", nil)
				return
			}
			next(w, r.WithContext(httpx.ContextWithUser(r.Context(), p)))
		}
	}
}

// OptionalAuth attaches a principal when present, without requiring one.
func (s *Service) OptionalAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if p, err := s.Lookup(r.Context(), r); err == nil {
			r = r.WithContext(httpx.ContextWithUser(r.Context(), p))
		}
		next(w, r)
	}
}

package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type ctxKey int

const ctxRequestID ctxKey = iota
const ctxUser ctxKey = iota + 1

// RequestIDFrom extracts the correlation ID for a request.
func RequestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxRequestID).(string); ok {
		return v
	}
	return ""
}

func newRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// WithRequestID attaches/propagates the X-Request-ID header.
func WithRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" || len(id) > 64 {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxRequestID, id)))
	})
}

// SecurityHeaders sets the defensive header baseline.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

// Recover converts panics into 500s with structured logs (no stack leak).
func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.ErrorContext(r.Context(), "panic recovered",
						slog.String("request_id", RequestIDFrom(r.Context())),
						slog.String("path", r.URL.Path),
						slog.Any("panic", rec))
					Fail(w, r, http.StatusInternalServerError, "internal_error",
						"Something went wrong. Please try again.", nil)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logging emits one structured line per request. Bodies are never logged.
func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			log.InfoContext(r.Context(), "http",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", rw.status),
				slog.Int64("ms", time.Since(start).Milliseconds()))
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// OriginCheck is CSRF defence-in-depth for cookie-authenticated mutations:
// state-changing requests must carry an Origin/Referer on the allowlist, or
// none at all (same-origin requests through the proxy omit nothing dangerous).
// Simple GETs are not affected.
func OriginCheck(allowedOrigins []string) func(http.Handler) http.Handler {
	allowed := map[string]bool{}
	for _, o := range allowedOrigins {
		allowed[strings.TrimRight(strings.ToLower(o), "/")] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}
			origin := strings.TrimRight(strings.ToLower(r.Header.Get("Origin")), "/")
			if origin != "" && !allowed[origin] {
				referer := r.Header.Get("Referer")
				if referer == "" || !originAllowedFromReferer(referer, allowed) {
					Fail(w, r, http.StatusForbidden, "forbidden",
						"Request origin is not allowed.", nil)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func originAllowedFromReferer(referer string, allowed map[string]bool) bool {
	idx := strings.Index(referer, "://")
	if idx < 0 {
		return false
	}
	rest := referer[idx+3:]
	host := rest
	if i := strings.Index(rest, "/"); i >= 0 {
		host = rest[:i]
	}
	// referer scheme is lost; accept host:port match against allowlist entries
	for o := range allowed {
		if strings.TrimPrefix(strings.TrimPrefix(o, "http://"), "https://") == host {
			return true
		}
	}
	return false
}

// RateLimiter is a fixed-window limiter. Local development uses the in-memory
// implementation; Redis-backed limiting can be swapped in via the same interface.
type RateLimiter interface {
	Allow(key string, limit int, window time.Duration) bool
}

// MemoryRateLimiter is process-local; sufficient for a single-process demo.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
}

func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{buckets: map[string]*rateBucket{}}
}

type rateBucket struct {
	windowStart time.Time
	count       int
}

func (m *MemoryRateLimiter) Allow(key string, limit int, window time.Duration) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	b, ok := m.buckets[key]
	if !ok || now.Sub(b.windowStart) >= window {
		if len(m.buckets) > 10_000 { // simple safety valve
			m.buckets = map[string]*rateBucket{}
		}
		m.buckets[key] = &rateBucket{windowStart: now, count: 1}
		return true
	}
	if b.count >= limit {
		return false
	}
	b.count++
	return true
}

// RateLimit wraps a handler with per-key limits for sensitive routes.
func RateLimit(limiter RateLimiter, keyFn func(r *http.Request) string, limit int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(keyFn(r), limit, window) {
			w.Header().Set("Retry-After", "60")
			Fail(w, r, http.StatusTooManyRequests, "rate_limited",
				"Too many requests. Please slow down.", nil)
			return
		}
		next(w, r)
	}
}

// KeyByIP identifies the caller by the last hop (proxy-aware for localhost dev).
func KeyByIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// KeyByUser identifies an authenticated caller; falls back to IP.
func KeyByUser(userID string) func(r *http.Request) string {
	return func(r *http.Request) string {
		if u := UserFrom(r.Context()); u != nil && u.ID == userID {
			return "user:" + userID
		}
		return "ip:" + KeyByIP(r)
	}
}

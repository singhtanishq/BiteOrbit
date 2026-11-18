package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/httpx"
)

const (
	sessionTTL    = 7 * 24 * time.Hour
	sessionCookie = "bo_session"
	absoluteTTL   = 30 * 24 * time.Hour
)

var ErrNoSession = errors.New("no session")

// Service manages session records and the session cookie.
type Service struct {
	pool         *pgxpool.Pool
	cookieDomain string
	secureCookie bool
}

func NewService(pool *pgxpool.Pool, cookieDomain string, secureCookie bool) *Service {
	return &Service{pool: pool, cookieDomain: cookieDomain, secureCookie: secureCookie}
}

type sessionRow struct {
	id        string
	userID    string
	tokenHash []byte
	expiresAt time.Time
	createdAt time.Time
}

// Create starts a new session for the user and sets the cookie. The raw token
// exists only inside this call; only its SHA-256 is stored.
func (s *Service) Create(w http.ResponseWriter, r *http.Request, userID string) error {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return fmt.Errorf("generate session token: %w", err)
	}
	hash := sha256.Sum256(token)
	_, err := s.pool.Exec(r.Context(), `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent, last_seen_at)
		VALUES ($1, $2, $3, $4, $5, now())`,
		userID, hash[:], time.Now().Add(sessionTTL), nullableIP(r), truncateUA(r))
	if err != nil {
		return fmt.Errorf("insert session: %w", err)
	}
	s.setCookie(w, encodeToken(token), time.Now().Add(sessionTTL))
	return nil
}

// Refresh rotates the current session: the old row is revoked and linked to a
// brand-new one (reuse detection friendly), and the cookie is replaced.
func (s *Service) Refresh(w http.ResponseWriter, r *http.Request, principal *httpx.Principal) error {
	oldHash, ok := cookieHash(r)
	if !ok {
		return apperr.New(apperr.CodeUnauthorized, "No active session to refresh.")
	}
	var oldID string
	err := s.pool.QueryRow(r.Context(),
		`SELECT id FROM sessions WHERE token_hash = $1 AND revoked_at IS NULL AND expires_at > now()`,
		oldHash).Scan(&oldID)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.CodeUnauthorized, "Session is no longer valid.")
	}
	if err != nil {
		return err
	}
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return err
	}
	hash := sha256.Sum256(token)
	var newID string
	err = s.pool.QueryRow(r.Context(), `
		WITH new_session AS (
			INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent, last_seen_at)
			VALUES ($1, $2, $3, $4, $5, now())
			RETURNING id
		)
		UPDATE sessions SET revoked_at = now(), rotated_to = (SELECT id FROM new_session)
		WHERE id = $6
		RETURNING (SELECT id FROM new_session)`,
		principal.ID, hash[:], time.Now().Add(sessionTTL), nullableIP(r), truncateUA(r), oldID).Scan(&newID)
	if err != nil {
		return fmt.Errorf("rotate session: %w", err)
	}
	s.setCookie(w, encodeToken(token), time.Now().Add(sessionTTL))
	return nil
}

// Lookup resolves the cookie to a principal, loading roles. Expired/revoked
// sessions are treated as absent.
func (s *Service) Lookup(ctx context.Context, r *http.Request) (*httpx.Principal, error) {
	hash, ok := cookieHash(r)
	if !ok {
		return nil, ErrNoSession
	}
	var p httpx.Principal
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.name, u.email, COALESCE(array_agg(r.role) FILTER (WHERE r.role IS NOT NULL), '{}')
		FROM sessions s
		JOIN users u ON u.id = s.user_id AND u.status = 'active' AND u.deleted_at IS NULL
		LEFT JOIN user_roles r ON r.user_id = u.id
		WHERE s.token_hash = $1 AND s.revoked_at IS NULL AND s.expires_at > now()
		GROUP BY u.id, u.name, u.email`,
		hash).Scan(&p.ID, &p.Name, &p.Email, &p.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, err
	}
	_, _ = s.pool.Exec(ctx,
		`UPDATE sessions SET last_seen_at = now() WHERE token_hash = $1`, hash)
	return &p, nil
}

// Revoke clears the current session (logout).
func (s *Service) Revoke(w http.ResponseWriter, r *http.Request) error {
	hash, ok := cookieHash(r)
	if !ok {
		// Nothing to revoke; clear cookie anyway so the browser forgets it.
		s.clearCookie(w)
		return nil
	}
	_, err := s.pool.Exec(r.Context(),
		`UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, hash)
	s.clearCookie(w)
	return err
}

// RevokeAll logs the user out of every device.
func (s *Service) RevokeAll(ctx context.Context, userID string) error {
	_, err := s.pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func (s *Service) setCookie(w http.ResponseWriter, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Domain:   s.cookieDomain,
		Expires:  expires,
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func (s *Service) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		Domain:   s.cookieDomain,
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		Secure:   s.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})
}

func cookieHash(r *http.Request) ([]byte, bool) {
	c, err := r.Cookie(sessionCookie)
	if err != nil || c.Value == "" {
		return nil, false
	}
	raw, err := decodeToken(c.Value)
	if err != nil {
		return nil, false
	}
	h := sha256.Sum256(raw)
	return h[:], true
}

// Tokens travel as "v1.<hex>" so future formats can coexist.
func encodeToken(raw []byte) string { return "v1." + hex.EncodeToString(raw) }

func decodeToken(v string) ([]byte, error) {
	if len(v) < 3 || v[:3] != "v1." {
		return nil, fmt.Errorf("bad token format")
	}
	return hex.DecodeString(v[3:])
}

func nullableIP(r *http.Request) any {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	return ip
}

func truncateUA(r *http.Request) string {
	ua := r.UserAgent()
	if len(ua) > 300 {
		ua = ua[:300]
	}
	return ua
}

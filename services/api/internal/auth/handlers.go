package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/outbox"
)

// RegisterBody is the sign-up payload. Only customers self-register; other
// roles exist via seeding/admin grants.
type RegisterBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
	Phone    string `json:"phone"`
}

var phoneRe = regexp.MustCompile(`^\+?[0-9]{10,14}$`)

// Register creates a customer account and starts a session.
func (s *Service) Register(w http.ResponseWriter, r *http.Request) {
	var body RegisterBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))
	body.Name = strings.TrimSpace(body.Name)
	body.Phone = strings.TrimSpace(body.Phone)

	if body.Name == "" || len(body.Name) > 100 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Please provide your name.", nil)
		return
	}
	if _, err := mail.ParseAddress(body.Email); err != nil || len(body.Email) > 200 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Please provide a valid email address.", nil)
		return
	}
	if body.Phone != "" && !phoneRe.MatchString(body.Phone) {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Phone must be 10–14 digits, optionally with +.", nil)
		return
	}
	if err := PasswordPolicy(body.Password); err != nil {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}

	hash, err := HashPassword(body.Password)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var userID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (email, phone, name) VALUES ($1, NULLIF($2,''), $3) RETURNING id`,
		body.Email, body.Phone, body.Name).Scan(&userID)
	if err != nil {
		if isUniqueViolation(err) {
			httpx.Fail(w, r, http.StatusConflict, "conflict",
				"This email is already registered. Try signing in instead.", nil)
			return
		}
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(),
		`INSERT INTO user_roles (user_id, role) VALUES ($1, 'customer')`, userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(),
		`INSERT INTO credentials (user_id, password_hash) VALUES ($1, $2)`, userID, hash); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(),
		`INSERT INTO customer_preferences (user_id) VALUES ($1) ON CONFLICT DO NOTHING`, userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO auth_events (user_id, event_type, ip, user_agent)
		VALUES ($1, 'register', $2, $3)`, userID, clientIP(r), truncateUserAgent(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = outbox.Enqueue(r.Context(), tx, "user", userID, "user.registered",
		map[string]any{"user_id": userID, "email": body.Email}, nil); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err := s.Create(w, r, userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{
		"id": userID, "name": body.Name, "email": body.Email, "roles": []string{"customer"},
	})
}

type loginBody struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// Login is enumeration-safe: unknown email and wrong password produce the
// identical generic error, and failures are recorded without secrets.
func (s *Service) Login(w http.ResponseWriter, r *http.Request) {
	var body loginBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	body.Email = strings.ToLower(strings.TrimSpace(body.Email))

	var userID, hash string
	err := s.pool.QueryRow(r.Context(), `
		SELECT u.id, c.password_hash
		FROM users u JOIN credentials c ON c.user_id = u.id
		WHERE u.email = $1 AND u.status = 'active' AND u.deleted_at IS NULL`,
		body.Email).Scan(&userID, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		s.recordAuthEvent(r, "login_failed", nil)
		httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized", "Invalid email or password.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	ok, err := VerifyPassword(body.Password, hash)
	if err != nil || !ok {
		s.recordAuthEvent(r, "login_failed", &userID)
		httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized", "Invalid email or password.", nil)
		return
	}

	if _, err := s.pool.Exec(r.Context(), `
		INSERT INTO auth_events (user_id, event_type, ip, user_agent)
		VALUES ($1, 'login_success', $2, $3)`, userID, clientIP(r), truncateUserAgent(r)); err != nil {
		slog.Warn("auth event log failed", slog.Any("err", err))
	}
	if err := s.Create(w, r, userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var name, email string
	var roles []string
	_ = s.pool.QueryRow(r.Context(), `
		SELECT u.name, u.email, COALESCE(array_agg(r.role) FILTER (WHERE r.role IS NOT NULL), '{}')
		FROM users u LEFT JOIN user_roles r ON r.user_id = u.id
		WHERE u.id = $1 GROUP BY u.id, u.name, u.email`, userID).Scan(&name, &email, &roles)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"id": userID, "name": name, "email": email, "roles": roles,
	})
}

// Session returns the authenticated principal (401 when anonymous).
func (s *Service) Session(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if p == nil {
		httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized", "Not signed in.", nil)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, p)
}

// RefreshSession rotates the session token (old one is revoked and linked).
func (s *Service) RefreshSession(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if p == nil {
		httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized", "Not signed in.", nil)
		return
	}
	if err := s.Refresh(w, r, p); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s.recordAuthEvent(r, "session_rotated", &p.ID)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"refreshed": true})
}

// Logout revokes the current session.
func (s *Service) Logout(w http.ResponseWriter, r *http.Request) {
	if p := httpx.UserFrom(r.Context()); p != nil {
		s.recordAuthEvent(r, "logout", &p.ID)
	}
	if err := s.Revoke(w, r); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"logged_out": true})
}

// LogoutAll revokes every session for the caller (security control).
func (s *Service) LogoutAll(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if p == nil {
		httpx.Fail(w, r, http.StatusUnauthorized, "unauthorized", "Not signed in.", nil)
		return
	}
	if err := s.RevokeAll(r.Context(), p.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s.recordAuthEvent(r, "logout_all", &p.ID)
	s.clearCookie(w)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"logged_out": true})
}

// --- password reset -------------------------------------------------------

type resetRequestBody struct {
	Email string `json:"email"`
}

// PasswordResetRequest always answers identically (no account enumeration).
func (s *Service) PasswordResetRequest(w http.ResponseWriter, r *http.Request) {
	var body resetRequestBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))

	var userID string
	err := s.pool.QueryRow(r.Context(),
		`SELECT id FROM users WHERE email = $1 AND status = 'active' AND deleted_at IS NULL`, email).Scan(&userID)
	if err == nil {
		token := make([]byte, 24)
		if _, err := rand.Read(token); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		hash := sha256.Sum256(token)
		if _, err := s.pool.Exec(r.Context(), `
			INSERT INTO password_resets (user_id, token_hash, expires_at)
			VALUES ($1, $2, now() + interval '30 minutes')`, userID, hash[:]); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		s.recordAuthEvent(r, "password_reset_requested", &userID)
		raw := hex.EncodeToString(token)
		if err := s.email.Send(r.Context(), email,
			"BiteOrbit password reset",
			"Reset your BiteOrbit password with this token (valid 30 minutes):\n\n"+raw+
				"\n\nThis is a local demo email. If you did not request a reset, ignore this message.",
		); err != nil {
			slog.WarnContext(r.Context(), "password reset email failed", slog.Any("err", err))
		}
	}
	// Identical response whether or not the account exists.
	httpx.WriteData(w, r, http.StatusAccepted, map[string]any{
		"message": "If that email is registered, a reset link has been sent. Check your inbox (Mailpit in local demo).",
	})
}

type resetConfirmBody struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

// PasswordResetConfirm consumes a valid token, sets the new password and
// revokes all sessions (a reset must evict attackers).
func (s *Service) PasswordResetConfirm(w http.ResponseWriter, r *http.Request) {
	var body resetConfirmBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err := PasswordPolicy(body.NewPassword); err != nil {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", err.Error(), nil)
		return
	}
	raw, err := hex.DecodeString(strings.TrimSpace(body.Token))
	if err != nil || len(raw) != 24 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Invalid reset token.", nil)
		return
	}
	hash := sha256.Sum256(raw)

	var userID string
	err = s.pool.QueryRow(r.Context(), `
		UPDATE password_resets SET used_at = now()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > now()
		RETURNING user_id`, hash[:]).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Reset token is invalid or expired. Request a new one.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	newHash, err := HashPassword(body.NewPassword)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(),
		`UPDATE credentials SET password_hash = $2, password_updated_at = now(), must_change = false WHERE user_id = $1`,
		userID, newHash); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(),
		`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO auth_events (user_id, event_type, ip, user_agent)
		VALUES ($1, 'password_reset_completed', $2, $3)`, userID, clientIP(r), truncateUserAgent(r)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	s.clearCookie(w)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"password_updated": true})
}

// --- helpers --------------------------------------------------------------

// recordAuthEvent logs a security-relevant authentication event. userID is
// optional (nil for anonymous failures). Never logs secrets.
func (s *Service) recordAuthEvent(r *http.Request, eventType string, userID *string) {
	var uid any
	if userID != nil {
		uid = *userID
	}
	if _, err := s.pool.Exec(r.Context(), `
		INSERT INTO auth_events (user_id, event_type, ip, user_agent)
		VALUES ($1, $2, $3, $4)`, uid, eventType, clientIP(r), truncateUserAgent(r)); err != nil {
		slog.WarnContext(r.Context(), "auth event log failed", slog.Any("err", err))
	}
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func clientIP(r *http.Request) any {
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	if host == "" {
		return nil
	}
	return host
}

func truncateUserAgent(r *http.Request) string {
	ua := r.UserAgent()
	if len(ua) > 300 {
		return ua[:300]
	}
	return ua
}

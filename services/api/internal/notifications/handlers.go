// Package notifications implements the in-app notification centre and
// per-category delivery preferences. Email sending respects preferences via
// the notification worker.
package notifications

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// List serves GET /notifications?unread_only=true
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	where := "user_id = $1"
	if r.URL.Query().Get("unread_only") == "true" {
		where += " AND read_at IS NULL"
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, category, title, body, read_at, created_at
		FROM notifications WHERE `+where+`
		ORDER BY created_at DESC LIMIT 50`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type n struct {
		ID        string  `json:"id"`
		Category  string  `json:"category"`
		Title     string  `json:"title"`
		Body      string  `json:"body"`
		ReadAt    *string `json:"read_at"`
		CreatedAt string  `json:"created_at"`
	}
	out := []n{}
	unread := 0
	for rows.Next() {
		var v n
		if rows.Scan(&v.ID, &v.Category, &v.Title, &v.Body, &v.ReadAt, &v.CreatedAt) == nil {
			if v.ReadAt == nil {
				unread++
			}
			out = append(out, v)
		}
	}
	httpx.WriteMeta(w, r, out, map[string]any{"unread": unread})
}

// MarkRead serves POST /notifications/{id}/read (owner-only).
func (h *Handlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = now() WHERE id = $2 AND user_id = $1 AND read_at IS NULL`, p.ID, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.WriteData(w, r, http.StatusOK, map[string]any{"already_read": true})
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"read": true})
}

// MarkAllRead serves POST /notifications/read-all.
func (h *Handlers) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	_, err := h.pool.Exec(r.Context(),
		`UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"read": true})
}

// Preferences serves GET /notifications/preferences.
func (h *Handlers) Preferences(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT category, inapp, email, sms FROM notification_preferences WHERE user_id = $1`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type pref struct {
		Category string `json:"category"`
		InApp    bool   `json:"inapp"`
		Email    bool   `json:"email"`
		SMS      bool   `json:"sms"`
	}
	out := []pref{}
	for rows.Next() {
		var v pref
		if rows.Scan(&v.Category, &v.InApp, &v.Email, &v.SMS) == nil {
			out = append(out, v)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type prefBody struct {
	Category string `json:"category"`
	InApp    *bool  `json:"inapp"`
	Email    *bool  `json:"email"`
	SMS      *bool  `json:"sms"`
}

// UpdatePreference serves PATCH /notifications/preferences — upsert per category.
func (h *Handlers) UpdatePreference(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body prefBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// start from current row (or defaults)
	var inapp, email, sms = true, true, false
	var exists bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT EXISTS(SELECT 1 FROM notification_preferences WHERE user_id=$1 AND category=$2)`,
		p.ID, body.Category).Scan(&exists)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if exists {
		_ = h.pool.QueryRow(r.Context(), `
			SELECT inapp, email, sms FROM notification_preferences WHERE user_id=$1 AND category=$2`,
			p.ID, body.Category).Scan(&inapp, &email, &sms)
	}
	if body.InApp != nil {
		inapp = *body.InApp
	}
	if body.Email != nil {
		email = *body.Email
	}
	if body.SMS != nil {
		sms = *body.SMS
	}
	if _, err = h.pool.Exec(r.Context(), `
		INSERT INTO notification_preferences (user_id, category, inapp, email, sms)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id, category) DO UPDATE SET inapp=$3, email=$4, sms=$5`,
		p.ID, body.Category, inapp, email, sms); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"category": body.Category, "inapp": inapp, "email": email, "sms": sms,
	})
}

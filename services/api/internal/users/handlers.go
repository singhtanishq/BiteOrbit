// Package users implements customer profile, preferences and address
// management. Every query is scoped to the authenticated user — record-level
// authorisation is enforced in SQL, not in the client.
package users

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// Me returns the caller's profile and preferences.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var out struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Email       string   `json:"email"`
		Phone       *string  `json:"phone"`
		Roles       []string `json:"roles"`
		Preferences struct {
			IsVegPref         *bool    `json:"is_veg_pref"`
			PreferredCuisines []string `json:"preferred_cuisines"`
			Language          string   `json:"language"`
		} `json:"preferences"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT u.id, u.name, u.email, u.phone,
			cp.is_veg_pref,
			COALESCE(cp.preferred_cuisines, '{}'),
			COALESCE(cp.language, 'en')
		FROM users u
		LEFT JOIN customer_preferences cp ON cp.user_id = u.id
		WHERE u.id = $1`, p.ID).Scan(&out.ID, &out.Name, &out.Email, &out.Phone,
		&out.Preferences.IsVegPref, &out.Preferences.PreferredCuisines, &out.Preferences.Language)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// Roles already carried in the principal; re-read for freshness.
	out.Roles = p.Roles
	httpx.WriteData(w, r, http.StatusOK, out)
}

type updateMeBody struct {
	Name  *string `json:"name"`
	Phone *string `json:"phone"`
}

// UpdateMe patches profile fields with validation.
func (h *Handlers) UpdateMe(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body updateMeBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.Name != nil {
		name := strings.TrimSpace(*body.Name)
		if name == "" || len(name) > 100 {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
				"Name must be 1–100 characters.", nil)
			return
		}
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE users SET name = $2, updated_at = now() WHERE id = $1`, p.ID, name); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	if body.Phone != nil {
		phone := strings.TrimSpace(*body.Phone)
		if phone != "" && !phoneRe.MatchString(phone) {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
				"Phone must be 10–14 digits, optionally with +.", nil)
			return
		}
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE users SET phone = NULLIF($2,''), updated_at = now() WHERE id = $1`, p.ID, phone); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"updated": true})
}

var phoneRe = regexp.MustCompile(`^\+?[0-9]{10,14}$`)
var pincodeRe = regexp.MustCompile(`^[1-9][0-9]{5}$`)

// --- addresses ------------------------------------------------------------

type addressBody struct {
	Label                string   `json:"label"`
	RecipientName        string   `json:"recipient_name"`
	RecipientPhone       string   `json:"recipient_phone"`
	Line1                string   `json:"line1"`
	Line2                *string  `json:"line2"`
	Landmark             *string  `json:"landmark"`
	Locality             string   `json:"locality"`
	City                 string   `json:"city"`
	State                string   `json:"state"`
	PostalCode           string   `json:"postal_code"`
	Lat                  *float64 `json:"lat"`
	Lng                  *float64 `json:"lng"`
	DeliveryInstructions *string  `json:"delivery_instructions"`
	IsDefault            bool     `json:"is_default"`
}

func validateAddress(body addressBody) string {
	switch {
	case body.RecipientName == "" || len(body.RecipientName) > 100:
		return "recipient_name is required (max 100 chars)."
	case !phoneRe.MatchString(body.RecipientPhone):
		return "recipient_phone must be 10–14 digits."
	case body.Line1 == "" || len(body.Line1) > 200:
		return "line1 is required (max 200 chars)."
	case body.Locality == "" || body.City == "" || body.State == "":
		return "locality, city and state are required."
	case !pincodeRe.MatchString(body.PostalCode):
		return "postal_code must be a 6-digit Indian pincode."
	case body.Label != "home" && body.Label != "work" && body.Label != "other":
		return "label must be home, work or other."
	case (body.Lat == nil) != (body.Lng == nil):
		return "lat and lng must be provided together."
	}
	if body.Lat != nil && (outside(*body.Lat, -90, 90) || outside(*body.Lng, -180, 180)) {
		return "lat/lng out of range."
	}
	return ""
}

func outside(v, lo, hi float64) bool { return v < lo || v > hi }

// ListAddresses returns the caller's addresses (newest default first).
func (h *Handlers) ListAddresses(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, label, recipient_name, recipient_phone, line1, line2, landmark,
			locality, city, state, postal_code, lat, lng, delivery_instructions, is_default
		FROM customer_addresses
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY is_default DESC, created_at DESC`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type addr struct {
		ID                   string   `json:"id"`
		Label                string   `json:"label"`
		RecipientName        string   `json:"recipient_name"`
		RecipientPhone       string   `json:"recipient_phone"`
		Line1                string   `json:"line1"`
		Line2                *string  `json:"line2"`
		Landmark             *string  `json:"landmark"`
		Locality             string   `json:"locality"`
		City                 string   `json:"city"`
		State                string   `json:"state"`
		PostalCode           string   `json:"postal_code"`
		Lat                  *float64 `json:"lat"`
		Lng                  *float64 `json:"lng"`
		DeliveryInstructions *string  `json:"delivery_instructions"`
		IsDefault            bool     `json:"is_default"`
	}
	out := []addr{}
	for rows.Next() {
		var a addr
		if err := rows.Scan(&a.ID, &a.Label, &a.RecipientName, &a.RecipientPhone,
			&a.Line1, &a.Line2, &a.Landmark, &a.Locality, &a.City, &a.State,
			&a.PostalCode, &a.Lat, &a.Lng, &a.DeliveryInstructions, &a.IsDefault); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, a)
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// CreateAddress validates and inserts an address for the caller.
func (h *Handlers) CreateAddress(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body addressBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if msg := validateAddress(body); msg != "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", msg, nil)
		return
	}
	if body.Label == "" {
		body.Label = "other"
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if body.IsDefault {
		if _, err = tx.Exec(r.Context(),
			`UPDATE customer_addresses SET is_default = false WHERE user_id = $1`, p.ID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	var id string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO customer_addresses
		(user_id, label, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, delivery_instructions, is_default)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,
			$15 OR NOT EXISTS (SELECT 1 FROM customer_addresses WHERE user_id = $1 AND deleted_at IS NULL))
		RETURNING id`,
		p.ID, body.Label, body.RecipientName, body.RecipientPhone, body.Line1,
		body.Line2, body.Landmark, body.Locality, body.City, body.State,
		body.PostalCode, body.Lat, body.Lng, body.DeliveryInstructions, body.IsDefault).Scan(&id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"id": id})
}

// UpdateAddress patches an address owned by the caller. Another user's
// address id resolves to 404 — never 403 — to avoid confirming existence.
func (h *Handlers) UpdateAddress(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")

	var body addressBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if msg := validateAddress(body); msg != "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", msg, nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if body.IsDefault {
		if _, err = tx.Exec(r.Context(),
			`UPDATE customer_addresses SET is_default = false WHERE user_id = $1`, p.ID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE customer_addresses SET
			label = $3, recipient_name = $4, recipient_phone = $5, line1 = $6, line2 = $7,
			landmark = $8, locality = $9, city = $10, state = $11, postal_code = $12,
			lat = $13, lng = $14, delivery_instructions = $15, is_default = $16, updated_at = now()
		WHERE id = $2 AND user_id = $1 AND deleted_at IS NULL`,
		p.ID, id, body.Label, body.RecipientName, body.RecipientPhone, body.Line1,
		body.Line2, body.Landmark, body.Locality, body.City, body.State,
		body.PostalCode, body.Lat, body.Lng, body.DeliveryInstructions, body.IsDefault)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Address not found.", nil)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"updated": true})
}

// DeleteAddress soft-deletes an address owned by the caller.
func (h *Handlers) DeleteAddress(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE customer_addresses SET deleted_at = now(), is_default = false
		WHERE id = $2 AND user_id = $1 AND deleted_at IS NULL`, p.ID, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Address not found.", nil)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"deleted": true})
}

// SetDefaultAddress makes the given address the caller's default.
func (h *Handlers) SetDefaultAddress(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(),
		`UPDATE customer_addresses SET is_default = false WHERE user_id = $1`, p.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	tag, err := tx.Exec(r.Context(), `
		UPDATE customer_addresses SET is_default = true, updated_at = now()
		WHERE id = $2 AND user_id = $1 AND deleted_at IS NULL`, p.ID, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Address not found.", nil)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"default_set": true})
}

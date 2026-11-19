// Package reviews implements P1 customer feedback: one review per delivered
// order (owner-only), public per-outlet listing, and abuse reporting. Demo
// reviews seeded into the dataset are labelled synthetic there; live reviews
// come only from real (demo) orders.
package reviews

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/httpx"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

// Create serves POST /reviews — one review per delivered order.
type createBody struct {
	OrderID        string `json:"order_id"`
	Rating         int    `json:"rating"`
	DeliveryRating int    `json:"delivery_rating"`
	Comment        string `json:"comment"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body createBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.Rating < 1 || body.Rating > 5 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Rating must be 1–5.", nil)
		return
	}
	if body.DeliveryRating < 0 || body.DeliveryRating > 5 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Delivery rating must be 0–5 (0 = not rated).", nil)
		return
	}
	if len(body.Comment) > 2000 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Comments are limited to 2000 characters.", nil)
		return
	}

	// eligibility: the caller's own DELIVERED order, not already reviewed
	var outletID string
	var already bool
	err := h.pool.QueryRow(r.Context(), `
		SELECT o.outlet_id,
			EXISTS(SELECT 1 FROM reviews rv WHERE rv.order_id = o.id)
		FROM orders o
		WHERE o.id = $1 AND o.customer_id = $2 AND o.status = 'delivered'`,
		body.OrderID, p.ID).Scan(&outletID, &already)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
			"Only your own delivered orders can be reviewed.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if already {
		httpx.Fail(w, r, http.StatusConflict, "conflict",
			"This order already has a review.", nil)
		return
	}

	deliveryRating := body.DeliveryRating
	if deliveryRating == 0 {
		deliveryRating = body.Rating
	}
	var reviewID string
	err = h.pool.QueryRow(r.Context(), `
		INSERT INTO reviews (order_id, customer_id, outlet_id, rating, delivery_rating, comment, status)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''),'published')
		RETURNING id`,
		body.OrderID, p.ID, outletID, body.Rating, deliveryRating, body.Comment).Scan(&reviewID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"id": reviewID, "outlet_id": outletID})
}

// ListForOutlet serves GET /restaurants/{id}/reviews — published reviews,
// newest first, paginated.
func (h *Handlers) ListForOutlet(w http.ResponseWriter, r *http.Request) {
	outletID := r.PathValue("id")
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	limit := atoiDefault(r.URL.Query().Get("limit"), 10)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 10
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT rv.rating, rv.delivery_rating, rv.comment, rv.created_at, u.name
		FROM reviews rv JOIN users u ON u.id = rv.customer_id
		WHERE rv.outlet_id = $1 AND rv.status = 'published'
		ORDER BY rv.created_at DESC
		LIMIT $2 OFFSET $3`, outletID, limit, (page-1)*limit)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type reviewRow struct {
		Rating         int       `json:"rating"`
		DeliveryRating int       `json:"delivery_rating"`
		Comment        *string   `json:"comment"`
		CreatedAt      time.Time `json:"created_at"`
		Customer       string    `json:"customer_name"`
	}
	out := []reviewRow{}
	for rows.Next() {
		var v reviewRow
		if rows.Scan(&v.Rating, &v.DeliveryRating, &v.Comment, &v.CreatedAt, &v.Customer) == nil {
			out = append(out, v)
		}
	}
	var total int
	_ = h.pool.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM reviews WHERE outlet_id = $1 AND status = 'published'`, outletID).Scan(&total)
	httpx.WriteMeta(w, r, out, map[string]any{"page": page, "limit": limit, "total": total})
}

// Report serves POST /reviews/{id}/report — abuse reporting for moderation.
func (h *Handlers) Report(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Please describe the problem (min 4 characters).", nil)
		return
	}
	var exists bool
	if err := h.pool.QueryRow(r.Context(),
		`SELECT EXISTS(SELECT 1 FROM reviews WHERE id = $1)`, id).Scan(&exists); err != nil || !exists {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Review not found.", nil)
		return
	}
	if _, err := h.pool.Exec(r.Context(),
		`INSERT INTO review_reports (review_id, reporter_id, reason) VALUES ($1,$2,$3)`,
		id, p.ID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{
		"reported": true,
		"message":  "Thanks — our moderation team will review this (demo).",
	})
}

// Mine serves GET /me/reviews — the caller's reviews.
func (h *Handlers) Mine(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT rv.id, rv.rating, rv.comment, rv.created_at, ot.name, o.reference
		FROM reviews rv JOIN outlets ot ON ot.id = rv.outlet_id
		JOIN orders o ON o.id = rv.order_id
		WHERE rv.customer_id = $1 ORDER BY rv.created_at DESC LIMIT 50`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type mineRow struct {
		ID        string    `json:"id"`
		Rating    int       `json:"rating"`
		Comment   *string   `json:"comment"`
		CreatedAt time.Time `json:"created_at"`
		Outlet    string    `json:"outlet_name"`
		Reference string    `json:"order_reference"`
	}
	out := []mineRow{}
	for rows.Next() {
		var v mineRow
		if rows.Scan(&v.ID, &v.Rating, &v.Comment, &v.CreatedAt, &v.Outlet, &v.Reference) == nil {
			out = append(out, v)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

func atoiDefault(s string, def int) int {
	n := 0
	if s == "" {
		return def
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

var _ = apperr.CodeForbidden

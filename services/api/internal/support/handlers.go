// Package support implements customer-facing support tickets: creation
// (optionally order-linked), the owner-only thread view and replies. Internal
// agent notes are never exposed through these endpoints.
package support

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

type createTicketBody struct {
	Category string `json:"category"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	OrderID  string `json:"order_id"`
}

var validCategories = map[string]bool{
	"missing_item": true, "wrong_item": true, "quality": true, "late_delivery": true,
	"payment": true, "cancellation_refund": true, "conduct": true, "account": true, "other": true,
}

// CreateTicket serves POST /support/tickets.
func (h *Handlers) CreateTicket(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body createTicketBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !validCategories[body.Category] {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Please choose a valid category.", nil)
		return
	}
	if len(body.Subject) < 4 || len(body.Subject) > 150 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Subject must be 4–150 characters.", nil)
		return
	}
	if len(body.Body) < 10 || len(body.Body) > 4000 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Description must be 10–4000 characters.", nil)
		return
	}

	// order must belong to the caller when provided
	var orderRef *string
	if body.OrderID != "" {
		err := h.pool.QueryRow(r.Context(),
			`SELECT reference FROM orders WHERE id = $1 AND customer_id = $2`,
			body.OrderID, p.ID).Scan(&orderRef)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
				"Order not found for this account.", nil)
			return
		}
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var ticketID, reference string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO support_tickets (reference, customer_id, order_id, category, subject)
		VALUES ('SUP-' || upper(substr(replace(gen_random_uuid()::text,'-',''),1,6)), $1, NULLIF($2,'')::uuid, $3, $4)
		RETURNING id, reference`,
		p.ID, body.OrderID, body.Category, body.Subject).Scan(&ticketID, &reference)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO support_messages (ticket_id, author_type, author_id, body)
		VALUES ($1,'customer',$2,$3)`, ticketID, p.ID, body.Body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{
		"id": ticketID, "reference": reference,
		"message": "Ticket created. Agents typically respond within one demo business day.",
	})
}

// ListMine serves GET /support/tickets — the caller's own tickets.
func (h *Handlers) ListMine(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	rows, err := h.pool.Query(r.Context(), `
		SELECT t.id, t.reference, t.category, t.subject, t.status, t.priority, t.created_at, t.updated_at,
			(SELECT COUNT(*) FROM support_messages m WHERE m.ticket_id = t.id)
		FROM support_tickets t WHERE t.customer_id = $1
		ORDER BY t.updated_at DESC LIMIT 50`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type ticketRow struct {
		ID        string    `json:"id"`
		Reference string    `json:"reference"`
		Category  string    `json:"category"`
		Subject   string    `json:"subject"`
		Status    string    `json:"status"`
		Priority  string    `json:"priority"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
		Replies   int       `json:"replies"`
	}
	out := []ticketRow{}
	for rows.Next() {
		var t ticketRow
		if rows.Scan(&t.ID, &t.Reference, &t.Category, &t.Subject, &t.Status, &t.Priority,
			&t.CreatedAt, &t.UpdatedAt, &t.Replies) == nil {
			out = append(out, t)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// GetThread serves GET /support/tickets/{id} — owner-only, WITHOUT internal
// notes (author-side filtering keeps agent notes out of customer views).
func (h *Handlers) GetThread(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")

	var t struct {
		ID        string    `json:"id"`
		Reference string    `json:"reference"`
		Category  string    `json:"category"`
		Subject   string    `json:"subject"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, reference, category, subject, status, created_at
		FROM support_tickets WHERE id = $1 AND customer_id = $2`, id, p.ID).Scan(
		&t.ID, &t.Reference, &t.Category, &t.Subject, &t.Status, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Ticket not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type msgRow struct {
		ID        string    `json:"id"`
		Author    string    `json:"author_type"`
		Body      string    `json:"body"`
		CreatedAt time.Time `json:"created_at"`
	}
	messages := []msgRow{}
	mrows, err := h.pool.Query(r.Context(), `
		SELECT id, author_type, body, created_at
		FROM support_messages
		WHERE ticket_id = $1 AND is_internal_note = false
		ORDER BY created_at, id`, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for mrows.Next() {
		var m msgRow
		if err := mrows.Scan(&m.ID, &m.Author, &m.Body, &m.CreatedAt); err != nil {
			mrows.Close()
			httpx.WriteError(w, r, err)
			return
		}
		messages = append(messages, m)
	}
	if err := mrows.Err(); err != nil {
		mrows.Close()
		httpx.WriteError(w, r, err)
		return
	}
	mrows.Close()

	httpx.WriteData(w, r, http.StatusOK, map[string]any{"ticket": t, "messages": messages})
}

type replyBody struct {
	Body string `json:"body"`
}

// Reply serves POST /support/tickets/{id}/messages — customer replies.
func (h *Handlers) Reply(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	var body replyBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Body) < 2 || len(body.Body) > 4000 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Message length must be 2–4000 characters.", nil)
		return
	}
	var ticketID string
	err := h.pool.QueryRow(r.Context(), `
		SELECT id FROM support_tickets WHERE id = $1 AND customer_id = $2`, id, p.ID).Scan(&ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Ticket not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = h.pool.Exec(r.Context(), `
		INSERT INTO support_messages (ticket_id, author_type, author_id, body)
		VALUES ($1,'customer',$2,$3)`, ticketID, p.ID, body.Body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// back to the agent queue
	if _, err = h.pool.Exec(r.Context(),
		`UPDATE support_tickets SET status='open', updated_at=now() WHERE id=$1`, ticketID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"replied": true})
}

var _ = apperr.CodeForbidden

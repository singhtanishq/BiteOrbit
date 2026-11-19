package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/httpx"
)

// ListTickets serves GET /admin/support/tickets?status=
func (h *Handlers) ListTickets(w http.ResponseWriter, r *http.Request) {
	where := "TRUE"
	args := []any{}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where = "t.status = $1"
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT t.id, t.reference, t.category, t.subject, t.status, t.priority,
			COALESCE(u.email, ''), t.created_at, t.updated_at
		FROM support_tickets t LEFT JOIN users u ON u.id = t.customer_id
		WHERE `+where+`
		ORDER BY CASE t.priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
			t.created_at DESC LIMIT 100`, args...)
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
		Customer  string    `json:"customer_email"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
	out := []ticketRow{}
	for rows.Next() {
		var t ticketRow
		if rows.Scan(&t.ID, &t.Reference, &t.Category, &t.Subject, &t.Status, &t.Priority,
			&t.Customer, &t.CreatedAt, &t.UpdatedAt) == nil {
			out = append(out, t)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// TicketDetail serves GET /admin/support/tickets/{id} — includes internal
// notes (agents only; never exposed to customers).
func (h *Handlers) TicketDetail(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var t struct {
		ID        string    `json:"id"`
		Reference string    `json:"reference"`
		Category  string    `json:"category"`
		Subject   string    `json:"subject"`
		Status    string    `json:"status"`
		Priority  string    `json:"priority"`
		OrderID   *string   `json:"order_id"`
		CreatedAt time.Time `json:"created_at"`
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT id, reference, category, subject, status, priority, order_id, created_at
		FROM support_tickets WHERE id = $1`, id).Scan(
		&t.ID, &t.Reference, &t.Category, &t.Subject, &t.Status, &t.Priority, &t.OrderID, &t.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Ticket not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type msgRow struct {
		ID        int64     `json:"id"`
		Author    string    `json:"author_type"`
		Body      string    `json:"body"`
		Internal  bool      `json:"is_internal_note"`
		CreatedAt time.Time `json:"created_at"`
	}
	messages := []msgRow{}
	mrows, err := h.pool.Query(r.Context(), `
		SELECT id, author_type, body, is_internal_note, created_at
		FROM support_messages WHERE ticket_id = $1 ORDER BY created_at, id`, id)
	if err == nil {
		for mrows.Next() {
			var m msgRow
			if mrows.Scan(&m.ID, &m.Author, &m.Body, &m.Internal, &m.CreatedAt) == nil {
				messages = append(messages, m)
			}
		}
		mrows.Close()
	}

	httpx.WriteData(w, r, http.StatusOK, map[string]any{"ticket": t, "messages": messages})
}

type replyBody struct {
	Body         string `json:"body"`
	InternalNote bool   `json:"internal_note"`
}

// ReplyTicket serves POST /admin/support/tickets/{id}/reply.
func (h *Handlers) ReplyTicket(w http.ResponseWriter, r *http.Request) {
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
	tag, err := h.pool.Exec(r.Context(), `
		INSERT INTO support_messages (ticket_id, author_type, author_id, body, is_internal_note)
		VALUES ($1,'agent',$2,$3,$4)`, id, p.ID, body.Body, body.InternalNote)
	if err != nil || tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Ticket not found.", nil)
		return
	}
	// non-internal replies move waiting tickets back to the customer
	if !body.InternalNote {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE support_tickets SET status = 'waiting_for_customer', updated_at = now() WHERE id = $1`, id)
	} else {
		_, _ = h.pool.Exec(r.Context(),
			`UPDATE support_tickets SET updated_at = now() WHERE id = $1`, id)
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{"replied": true})
}

type ticketStatusBody struct {
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

// UpdateTicket serves POST /admin/support/tickets/{id}/status.
func (h *Handlers) UpdateTicket(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body ticketStatusBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	sets := "updated_at = now()"
	args := []any{}
	add := func(name string, v any) {
		args = append(args, v)
		sets = name + " = $" + itoa(len(args)) + ", " + sets
	}
	validStatus := map[string]bool{"open": true, "assigned": true, "waiting_for_customer": true, "resolved": true, "closed": true}
	validPriority := map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}
	if body.Status != "" {
		if !validStatus[body.Status] {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Invalid status.", nil)
			return
		}
		add("status", body.Status)
		if body.Status == "resolved" || body.Status == "closed" {
			args = append(args, "now()")
			sets = "resolved_at = $" + itoa(len(args)) + ", " + sets
		}
	}
	if body.Priority != "" {
		if !validPriority[body.Priority] {
			httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Invalid priority.", nil)
			return
		}
		add("priority", body.Priority)
	}
	if len(args) == 0 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Nothing to update.", nil)
		return
	}
	args = append(args, id)
	_, err := h.pool.Exec(r.Context(),
		`UPDATE support_tickets SET `+sets+` WHERE id = $`+itoa(len(args)), args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"updated": true})
}

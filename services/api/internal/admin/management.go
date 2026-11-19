package admin

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"biteorbit/services/api/internal/httpx"
)

func clockNow() string { return time.Now().UTC().Format(time.RFC3339) }

// ListOrders serves GET /admin/orders?query=&status=&page= — full platform
// search. No hard deletes anywhere; interventions are audited.
func (h *Handlers) ListOrders(w http.ResponseWriter, r *http.Request) {
	page := atoiDefault(r.URL.Query().Get("page"), 1)
	limit := atoiDefault(r.URL.Query().Get("limit"), 20)
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}
	where := "TRUE"
	args := []any{}
	add := func(v any) string {
		args = append(args, v)
		return itoa(len(args))
	}
	if q := r.URL.Query().Get("query"); q != "" {
		p := "%" + q + "%"
		where += " AND (o.reference ILIKE $" + add(p) +
			" OR ot.name ILIKE $" + add(p) +
			" OR u.email ILIKE $" + add(p) + ")"
	}
	if s := r.URL.Query().Get("status"); s != "" {
		where += " AND o.status = $" + add(s)
	}
	args = append(args, limit, (page-1)*limit)

	rows, err := h.pool.Query(r.Context(), `
		SELECT o.id, o.reference, o.status, o.grand_total_paise, o.placed_at,
			ot.name, u.email,
			COALESCE((SELECT status FROM payments pm WHERE pm.order_id = o.id ORDER BY created_at DESC LIMIT 1), 'none')
		FROM orders o
		JOIN outlets ot ON ot.id = o.outlet_id
		JOIN users u ON u.id = o.customer_id
		WHERE `+where+`
		ORDER BY o.created_at DESC
		LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type row struct {
		ID         string    `json:"id"`
		Reference  string    `json:"reference"`
		Status     string    `json:"status"`
		GrandTotal int64     `json:"grand_total_paise"`
		PlacedAt   time.Time `json:"placed_at"`
		Outlet     string    `json:"outlet_name"`
		Customer   string    `json:"customer_email"`
		Payment    string    `json:"payment_status"`
	}
	out := []row{}
	for rows.Next() {
		var o row
		if err := rows.Scan(&o.ID, &o.Reference, &o.Status, &o.GrandTotal, &o.PlacedAt,
			&o.Outlet, &o.Customer, &o.Payment); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, o)
	}
	var total int
	_ = h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM orders o
		JOIN outlets ot ON ot.id = o.outlet_id
		JOIN users u ON u.id = o.customer_id WHERE `+where, args[:len(args)-2]...).Scan(&total)
	httpx.WriteMeta(w, r, out, map[string]any{"page": page, "limit": limit, "total": total})
}

// AdminCancel serves POST /admin/orders/{id}/cancel — audited administrative
// intervention with mandatory reason; refunds captured payments.
func (h *Handlers) AdminCancel(w http.ResponseWriter, r *http.Request) {
	roleOnly := isSupportOnly(httpx.UserFrom(r.Context()).Roles)
	if roleOnly {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden",
			"Support agents cannot cancel orders; escalate to operations.", nil)
		return
	}
	p := httpx.UserFrom(r.Context())
	orderID := r.PathValue("id")
	var body struct {
		Reason string `json:"reason"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"A reason is required for administrative cancellations.", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var from, customerID string
	err = tx.QueryRow(r.Context(), `
		SELECT status, customer_id FROM orders WHERE id = $1 FOR UPDATE`, orderID).Scan(&from, &customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch from {
	case "delivered", "rejected", "cancelled", "delivery_failed":
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			"Terminal orders cannot be cancelled; issue a refund instead.", nil)
		return
	}

	if _, err = tx.Exec(r.Context(), `
		UPDATE orders SET status='cancelled', cancel_reason=$2, cancelled_by_actor='admin', closed_at=now(), updated_at=now()
		WHERE id=$1`, orderID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
		VALUES ($1,'state_changed',$2,'cancelled','admin',$3,$4)`, orderID, from, p.ID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		UPDATE payments SET status = CASE WHEN method='cod' THEN 'cancelled' ELSE 'refunded' END, updated_at=now()
		WHERE order_id=$1 AND status IN ('captured','cod_due','pending')`, orderID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
		SELECT pm.id, pm.order_id, pm.amount_paise, 'processed', $2, pm.provider_reference || '-admin', 'admin', now()
		FROM payments pm WHERE pm.order_id = $1 AND pm.method <> 'cod' AND pm.status='refunded'`, orderID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = audit(r.Context(), tx, p.ID, "order.admin_cancel", "order", orderID,
		map[string]any{"from": from}, map[string]any{"status": "cancelled"}, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": "cancelled", "audited": true})
}

// ListUsers serves GET /admin/users?role=&query= — data-minimised view.
func (h *Handlers) ListUsers(w http.ResponseWriter, r *http.Request) {
	where := "u.deleted_at IS NULL"
	args := []any{}
	add := func(v any) string {
		args = append(args, v)
		return itoa(len(args))
	}
	if role := r.URL.Query().Get("role"); role != "" {
		where += " AND EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = u.id AND ur.role = $" + add(role) + ")"
	}
	if q := r.URL.Query().Get("query"); q != "" {
		p := "%" + q + "%"
		where += " AND (u.email ILIKE $" + add(p) + " OR u.name ILIKE $" + add(p) + ")"
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT u.id, u.name, u.email, u.status, u.created_at,
			COALESCE(array_agg(DISTINCT ur.role) FILTER (WHERE ur.role IS NOT NULL), '{}'),
			(SELECT COUNT(*) FROM orders o WHERE o.customer_id = u.id)
		FROM users u LEFT JOIN user_roles ur ON ur.user_id = u.id
		WHERE `+where+`
		GROUP BY u.id ORDER BY u.created_at DESC LIMIT 50`, args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type userRow struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Email     string    `json:"email"`
		Status    string    `json:"status"`
		CreatedAt time.Time `json:"created_at"`
		Roles     []string  `json:"roles"`
		Orders    int       `json:"orders"`
	}
	out := []userRow{}
	for rows.Next() {
		var u userRow
		if rows.Scan(&u.ID, &u.Name, &u.Email, &u.Status, &u.CreatedAt, &u.Roles, &u.Orders) == nil {
			out = append(out, u)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type userActionBody struct {
	Reason string `json:"reason"`
}

// SetUserStatus serves POST /admin/users/{id}/suspend | activate.
func (h *Handlers) userStatus(w http.ResponseWriter, r *http.Request, newStatus string) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden",
			"Support agents cannot change account status.", nil)
		return
	}
	id := r.PathValue("id")
	var body userActionBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "A reason is required.", nil)
		return
	}
	var before string
	err := h.pool.QueryRow(r.Context(), `SELECT status FROM users WHERE id = $1`, id).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "User not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if before == newStatus {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			"User is already "+newStatus+".", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(),
		`UPDATE users SET status=$2, updated_at=now() WHERE id=$1`, id, newStatus); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if newStatus == "suspended" {
		if _, err = tx.Exec(r.Context(),
			`UPDATE sessions SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, id); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	if err = audit(r.Context(), tx, p.ID, "user."+newStatus, "user", id,
		map[string]any{"status": before}, map[string]any{"status": newStatus}, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": newStatus})
}

func (h *Handlers) SuspendUser(w http.ResponseWriter, r *http.Request) {
	h.userStatus(w, r, "suspended")
}

func (h *Handlers) ActivateUser(w http.ResponseWriter, r *http.Request) {
	h.userStatus(w, r, "active")
}

// ListRestaurants serves GET /admin/restaurants — organisations, outlets and
// application status in one view.
func (h *Handlers) ListRestaurants(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT org.id, org.display_name, org.status, org.contact_email,
			COUNT(DISTINCT o.id) FILTER (WHERE o.deleted_at IS NULL),
			COALESCE((SELECT status FROM restaurant_applications ra WHERE ra.organisation_id = org.id ORDER BY ra.created_at DESC LIMIT 1), '')
		FROM restaurant_organisations org
		LEFT JOIN outlets o ON o.organisation_id = org.id
		GROUP BY org.id ORDER BY org.created_at DESC`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type orgRow struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Status    string `json:"status"`
		Email     string `json:"contact_email"`
		Outlets   int    `json:"outlets"`
		AppStatus string `json:"application_status"`
	}
	out := []orgRow{}
	for rows.Next() {
		var o orgRow
		if rows.Scan(&o.ID, &o.Name, &o.Status, &o.Email, &o.Outlets, &o.AppStatus) == nil {
			out = append(out, o)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// ListApplications serves GET /admin/applications?status=
func (h *Handlers) ListApplications(w http.ResponseWriter, r *http.Request) {
	where := "TRUE"
	args := []any{}
	if s := r.URL.Query().Get("status"); s != "" {
		args = append(args, s)
		where = "ra.status = $1"
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT ra.id, ra.status, org.display_name, ra.applicant_note, ra.created_at,
			COALESCE(reviewer.email, ''), COALESCE(ra.decision_note, '')
		FROM restaurant_applications ra
		JOIN restaurant_organisations org ON org.id = ra.organisation_id
		LEFT JOIN users reviewer ON reviewer.id = ra.reviewer_id
		WHERE `+where+`
		ORDER BY ra.created_at DESC LIMIT 50`, args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type appRow struct {
		ID        string    `json:"id"`
		Status    string    `json:"status"`
		Org       string    `json:"organisation"`
		Note      string    `json:"applicant_note"`
		CreatedAt time.Time `json:"created_at"`
		Reviewer  string    `json:"reviewer_email"`
		Decision  string    `json:"decision_note"`
	}
	out := []appRow{}
	for rows.Next() {
		var a appRow
		if rows.Scan(&a.ID, &a.Status, &a.Org, &a.Note, &a.CreatedAt, &a.Reviewer, &a.Decision) == nil {
			out = append(out, a)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type reviewBody struct {
	Decision string `json:"decision"` // approve | reject | changes_requested
	Note     string `json:"note"`
}

// ReviewApplication serves POST /admin/applications/{id}/review.
// Demo approvals are explicitly simulations (not legal verification).
func (h *Handlers) ReviewApplication(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden",
			"Support agents cannot review applications.", nil)
		return
	}
	id := r.PathValue("id")
	var body reviewBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var toStatus string
	switch body.Decision {
	case "approve":
		toStatus = "approved"
	case "reject":
		toStatus = "rejected"
	case "changes_requested":
		toStatus = "changes_requested"
	default:
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"decision must be approve, reject or changes_requested.", nil)
		return
	}
	if len(body.Note) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "A review note is required.", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	var from, orgID string
	err = tx.QueryRow(r.Context(), `
		SELECT status, organisation_id FROM restaurant_applications WHERE id = $1 FOR UPDATE`, id).Scan(&from, &orgID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Application not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if from == toStatus {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition", "Already in that state.", nil)
		return
	}

	if _, err = tx.Exec(r.Context(), `
		UPDATE restaurant_applications SET status=$2, reviewer_id=$3, decision_note=$4, decided_at=now(), updated_at=now()
		WHERE id=$1`, id, toStatus, p.ID, body.Note); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO application_events (application_id, from_status, to_status, actor_id, note)
		VALUES ($1,$2,$3,$4,$5)`, id, from, toStatus, p.ID, body.Note); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	// org status follows the application
	orgStatus := "approved"
	if toStatus == "rejected" {
		orgStatus = "rejected"
	} else if toStatus == "changes_requested" {
		orgStatus = "changes_requested"
	}
	if _, err = tx.Exec(r.Context(),
		`UPDATE restaurant_organisations SET status=$2, updated_at=now() WHERE id=$1`, orgID, orgStatus); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = audit(r.Context(), tx, p.ID, "application.review", "restaurant_application", id,
		map[string]any{"status": from}, map[string]any{"status": toStatus, "note": body.Note},
		"Demo review — simulation, not legal/licence verification."); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": toStatus, "simulated": true})
}

// ListRiders serves GET /admin/riders.
func (h *Handlers) ListRiders(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT dp.id, u.name, u.email, dp.status, dp.availability, dp.vehicle_type,
			COALESCE(dp.vehicle_number,''), COUNT(re.id)
		FROM delivery_partners dp
		JOIN users u ON u.id = dp.user_id
		LEFT JOIN rider_earnings re ON re.rider_id = dp.id
		GROUP BY dp.id, dp.created_at, u.name, u.email, dp.status, dp.availability, dp.vehicle_type, dp.vehicle_number
		ORDER BY dp.created_at DESC`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type riderRow struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Email        string `json:"email"`
		Status       string `json:"status"`
		Availability string `json:"availability"`
		Vehicle      string `json:"vehicle"`
		Number       string `json:"vehicle_number"`
		Deliveries   int    `json:"deliveries"`
	}
	out := []riderRow{}
	for rows.Next() {
		var o riderRow
		if rows.Scan(&o.ID, &o.Name, &o.Email, &o.Status, &o.Availability, &o.Vehicle, &o.Number, &o.Deliveries) == nil {
			out = append(out, o)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// RiderAction serves POST /admin/riders/{id}/approve | suspend — audited.
func (h *Handlers) riderStatus(w http.ResponseWriter, r *http.Request, newStatus string) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden",
			"Support agents cannot change rider status.", nil)
		return
	}
	id := r.PathValue("id")
	var body userActionBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "A reason is required.", nil)
		return
	}
	var before string
	if err := h.pool.QueryRow(r.Context(),
		`SELECT status FROM delivery_partners WHERE id = $1`, id).Scan(&before); err != nil {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Rider not found.", nil)
		return
	}
	if before == newStatus {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition", "Rider is already "+newStatus+".", nil)
		return
	}
	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(),
		`UPDATE delivery_partners SET status=$2, updated_at=now() WHERE id=$1`, id, newStatus); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if newStatus != "approved" {
		if _, err = tx.Exec(r.Context(),
			`UPDATE delivery_partners SET availability='offline' WHERE id=$1`, id); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	if err = audit(r.Context(), tx, p.ID, "rider."+newStatus, "delivery_partner", id,
		map[string]any{"status": before}, map[string]any{"status": newStatus}, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"status": newStatus})
}

func (h *Handlers) ApproveRider(w http.ResponseWriter, r *http.Request) {
	h.riderStatus(w, r, "approved")
}

func (h *Handlers) SuspendRider(w http.ResponseWriter, r *http.Request) {
	h.riderStatus(w, r, "suspended")
}

// ListAuditLogs serves GET /admin/audit-logs?target_type=&target_id=
func (h *Handlers) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	where := "TRUE"
	args := []any{}
	add := func(v any) string {
		args = append(args, v)
		return itoa(len(args))
	}
	if tt := r.URL.Query().Get("target_type"); tt != "" {
		where += " AND target_type = $" + add(tt)
	}
	if tid := r.URL.Query().Get("target_id"); tid != "" {
		where += " AND target_id = $" + add(tid)
	}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, actor_type, COALESCE(actor_id::text,''), action, target_type, target_id,
			before_state, after_state, reason, created_at
		FROM audit_logs WHERE `+where+` ORDER BY id DESC LIMIT 100`, args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type logRow struct {
		ID        int64     `json:"id"`
		ActorType string    `json:"actor_type"`
		ActorID   string    `json:"actor_id"`
		Action    string    `json:"action"`
		Target    string    `json:"target_type"`
		TargetID  string    `json:"target_id"`
		Before    *string   `json:"before_state"`
		After     *string   `json:"after_state"`
		Reason    *string   `json:"reason"`
		CreatedAt time.Time `json:"created_at"`
	}
	out := []logRow{}
	for rows.Next() {
		var l logRow
		if rows.Scan(&l.ID, &l.ActorType, &l.ActorID, &l.Action, &l.Target, &l.TargetID,
			&l.Before, &l.After, &l.Reason, &l.CreatedAt) == nil {
			out = append(out, l)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// FeatureFlags serves GET /admin/feature-flags; UpdateFlag PATCH path.
func (h *Handlers) FeatureFlags(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(),
		`SELECT key, enabled, description, updated_at FROM feature_flags ORDER BY key`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type flag struct {
		Key         string    `json:"key"`
		Enabled     bool      `json:"enabled"`
		Description string    `json:"description"`
		UpdatedAt   time.Time `json:"updated_at"`
	}
	out := []flag{}
	for rows.Next() {
		var f flag
		if rows.Scan(&f.Key, &f.Enabled, &f.Description, &f.UpdatedAt) == nil {
			out = append(out, f)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

type flagBody struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

// UpdateFlag serves PATCH /admin/feature-flags/{key}.
func (h *Handlers) UpdateFlag(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	if isSupportOnly(p.Roles) {
		httpx.Fail(w, r, http.StatusForbidden, "forbidden", "Support agents cannot change flags.", nil)
		return
	}
	key := r.PathValue("key")
	var body flagBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	var before bool
	err := h.pool.QueryRow(r.Context(),
		`SELECT enabled FROM feature_flags WHERE key = $1`, key).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Flag not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE feature_flags SET enabled=$2, updated_by=$3, updated_at=now() WHERE key=$1`,
		key, body.Enabled, p.ID)
	if err != nil || tag.RowsAffected() == 0 {
		httpx.WriteError(w, r, err)
		return
	}
	if err = audit(r.Context(), h.pool, p.ID, "flag.updated", "feature_flag", key,
		map[string]any{"enabled": before}, map[string]any{"enabled": body.Enabled}, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"key": key, "enabled": body.Enabled})
}

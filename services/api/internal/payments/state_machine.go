// Package payments owns the payment lifecycle: capture, failure/retry,
// webhook processing (signature-verified, idempotent, amount-matched) and
// refunds. Payment state is controlled exclusively by the server — a
// frontend success callback can never mark an order paid (see ADR and tests).
package payments

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/platform/payments"
)

type Handlers struct {
	pool     *pgxpool.Pool
	provider payments.Provider
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool, provider: payments.NewMock()}
}

// paymentState of an order's latest payment for the customer view.
type paymentDTO struct {
	ID          string     `json:"id"`
	OrderID     string     `json:"order_id"`
	Method      string     `json:"method"`
	Status      string     `json:"status"`
	AmountPaise int64      `json:"amount_paise"`
	Reference   *string    `json:"provider_reference,omitempty"`
	CapturedAt  *time.Time `json:"captured_at,omitempty"`
	Failure     *string    `json:"failure_reason,omitempty"`
	Simulated   bool       `json:"simulated"`
}

// ByOrder returns the caller's payment for an order (owner-only).
func (h *Handlers) ByOrder(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	orderID := r.PathValue("id")
	var dto paymentDTO
	err := h.pool.QueryRow(r.Context(), `
		SELECT pm.id, pm.order_id, pm.method, pm.status, pm.amount_paise, pm.provider_reference,
			pm.captured_at, pm.failure_reason
		FROM payments pm JOIN orders o ON o.id = pm.order_id
		WHERE pm.order_id = $1 AND o.customer_id = $2
		ORDER BY pm.created_at DESC LIMIT 1`, orderID, p.ID).Scan(
		&dto.ID, &dto.OrderID, &dto.Method, &dto.Status, &dto.AmountPaise,
		&dto.Reference, &dto.CapturedAt, &dto.Failure)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Payment not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	dto.Simulated = true // local mock provider is always simulated
	httpx.WriteData(w, r, http.StatusOK, dto)
}

type simulateBody struct {
	Outcome string `json:"outcome"` // success | failure
}

// Simulate is the LOCAL mock-gateway bridge: it mints a provider-signed
// webhook for the requested outcome and posts it through the same verified
// webhook path a real gateway would use. Available only with the mock
// provider (config.Validate forbids mock in production).
func (h *Handlers) Simulate(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	paymentID := r.PathValue("id")
	var body simulateBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.Outcome != "success" && body.Outcome != "failure" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"outcome must be success or failure.", nil)
		return
	}

	// caller must own the order; payment must be actionable
	var pay struct {
		id       string
		orderID  string
		ref      *string
		amount   int64
		status   string
		method   string
		orderRef string
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT pm.id, pm.order_id, pm.provider_reference, pm.amount_paise, pm.status, pm.method, o.reference
		FROM payments pm JOIN orders o ON o.id = pm.order_id
		WHERE pm.id = $1 AND o.customer_id = $2`, paymentID, p.ID).Scan(
		&pay.id, &pay.orderID, &pay.ref, &pay.amount, &pay.status, &pay.method, &pay.orderRef)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Payment not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if pay.method == "cod" {
		httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
			"Cash-on-delivery payments are settled at the door, not online.", nil)
		return
	}
	switch pay.status {
	case "pending", "requires_action":
	case "failed":
		// retry: create a fresh attempt row (attempt_no = n+1)
		var n int
		if err := h.pool.QueryRow(r.Context(),
			`SELECT COALESCE(MAX(attempt_no),0)+1 FROM payment_attempts WHERE payment_id = $1`, pay.id).Scan(&n); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err := h.pool.Exec(r.Context(), `
			INSERT INTO payment_attempts (payment_id, attempt_no, status, provider_reference, detail)
			VALUES ($1,$2,'retry_initiated',$3,'{"simulated":true}'::jsonb)`,
			pay.id, n, pay.ref); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err := h.pool.Exec(r.Context(),
			`UPDATE payments SET status = 'pending', failure_reason = NULL, updated_at = now() WHERE id = $1`, pay.id); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	default:
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			fmt.Sprintf("Payment is already %s.", pay.status), nil)
		return
	}

	// mint the signed webhook (exactly like a real provider notification)
	evtType := "payment.captured"
	if body.Outcome == "failure" {
		evtType = "payment.failed"
	}
	payload, _ := json.Marshal(map[string]any{
		"event_id":             fmt.Sprintf("evt_%s_%d", pay.id[:8], time.Now().UnixNano()),
		"type":                 evtType,
		"provider_payment_ref": deref(pay.ref),
		"order_reference":      pay.orderRef,
		"amount_paise":         pay.amount,
		"timestamp":            time.Now().Unix(),
	})
	signature := payments.SignMockWebhook(payload)

	// deliver to our own webhook endpoint over HTTP (same verification path
	// as production providers)
	base := baseURLFrom(r)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, base+"/api/v1/webhooks/mock",
		bytes.NewReader(payload))
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BiteOrbit-Signature", signature)
	req.Header.Set("X-BiteOrbit-Demo", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		httpx.Fail(w, r, http.StatusBadGateway, "service_unavailable",
			"The simulated gateway rejected the webhook.", nil)
		return
	}

	// report resulting state
	var status string
	_ = h.pool.QueryRow(r.Context(), `SELECT status FROM payments WHERE id = $1`, pay.id).Scan(&status)
	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"payment_id": pay.id,
		"status":     status,
		"simulated":  true,
		"demo_note":  "Processed through the signed webhook path, identical to a production provider integration.",
	})
}

// Webhook processes provider notifications: signature verification, replay
// protection (unique event id), amount/order matching and state transitions.
// Any mismatch leaves a rejected, non-processed row for auditing.
func (h *Handlers) Webhook(w http.ResponseWriter, r *http.Request) {
	body := httpx.LimitedBody(r, 1<<20)
	signature := r.Header.Get("X-BiteOrbit-Signature")

	evt, ok, err := h.provider.VerifyWebhook(body, signature)
	if err != nil || !ok {
		// record the rejected attempt for audit under a content hash id
		idHash := sha256.Sum256(body)
		_, _ = h.pool.Exec(r.Context(), `
			INSERT INTO payment_webhook_events (provider, event_id, payload, signature_valid, processing_error)
			VALUES ('mock', $1, $2::jsonb, false, $3)
			ON CONFLICT (provider, event_id) DO NOTHING`,
			"unverified-"+hex.EncodeToString(idHash[:8]), string(body), "signature verification failed")
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Webhook signature verification failed.", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// idempotency: unique (provider, event_id)
	var alreadyProcessed bool
	err = tx.QueryRow(r.Context(), `
		INSERT INTO payment_webhook_events (provider, event_id, payload, signature_valid)
		VALUES ('mock', $1, $2::jsonb, true)
		ON CONFLICT (provider, event_id) DO UPDATE SET provider = EXCLUDED.provider
		RETURNING processed_at IS NOT NULL`,
		evt.EventID, string(body)).Scan(&alreadyProcessed)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if alreadyProcessed {
		_ = tx.Commit(r.Context())
		httpx.WriteData(w, r, http.StatusOK, map[string]any{"deduplicated": true})
		return
	}

	// find the payment by provider reference and order reference
	var pay struct {
		id       string
		orderID  string
		orderRef string
		amount   int64
		status   string
		method   string
	}
	err = tx.QueryRow(r.Context(), `
		SELECT pm.id, pm.order_id, o.reference, pm.amount_paise, pm.status, pm.method
		FROM payments pm JOIN orders o ON o.id = pm.order_id
		WHERE pm.provider_reference = $1 AND o.reference = $2`,
		evt.ProviderPayRef, evt.OrderReference).Scan(
		&pay.id, &pay.orderID, &pay.orderRef, &pay.amount, &pay.status, &pay.method)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = tx.Exec(r.Context(), `
			UPDATE payment_webhook_events SET processing_error = 'payment not found for reference'
			WHERE provider='mock' AND event_id = $1`, evt.EventID)
		_ = tx.Commit(r.Context())
		httpx.Fail(w, r, http.StatusNotFound, "not_found",
			"No payment matches the webhook reference.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// amount/currency match — never trust the webhook amount blindly
	if evt.AmountPaise != pay.amount {
		_, _ = tx.Exec(r.Context(), `
			UPDATE payment_webhook_events SET processing_error = 'amount mismatch'
			WHERE provider='mock' AND event_id = $1`, evt.EventID)
		_ = tx.Commit(r.Context())
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Webhook amount does not match the payment.", nil)
		return
	}

	switch evt.Type {
	case "payment.captured":
		if pay.status != "pending" && pay.status != "requires_action" && pay.status != "failed" {
			_ = tx.Commit(r.Context())
			httpx.WriteData(w, r, http.StatusOK, map[string]any{"deduplicated": true, "status": pay.status})
			return
		}
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status='captured', captured_at=now(), updated_at=now() WHERE id=$1`, pay.id); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO payment_attempts (payment_id, attempt_no, status, provider_reference, detail)
			VALUES ($1, COALESCE((SELECT MAX(attempt_no)+1 FROM payment_attempts WHERE payment_id=$1),1),
				'captured', $2, '{"simulated":true,"via":"webhook"}'::jsonb)`,
			pay.id, evt.ProviderPayRef); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		// order: pending_payment → placed (only the server transitions this)
		if err = transitionOrder(r.Context(), tx, pay.orderID, "pending_payment", "placed", "system",
			"Payment captured via provider webhook"); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err = outbox.Enqueue(r.Context(), tx, "payment", pay.id, "payment.captured", map[string]any{
			"order_id": pay.orderID, "payment_id": pay.id, "amount_paise": pay.amount,
		}, ptrStr("payment-captured:"+pay.id)); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if err = outbox.Enqueue(r.Context(), tx, "order", pay.orderID, "order.status_changed", map[string]any{
			"order_id": pay.orderID, "customer_id": "", "reference": pay.orderRef,
			"from_state": "pending_payment", "to_state": "placed",
		}, ptrStr(fmt.Sprintf("order-status:%s:placed", pay.orderID))); err != nil {
			httpx.WriteError(w, r, err)
			return
		}

	case "payment.failed":
		if _, err = tx.Exec(r.Context(), `
			UPDATE payments SET status='failed', failure_reason='Provider reported failure (simulated)', updated_at=now()
			WHERE id=$1 AND status IN ('pending','requires_action')`, pay.id); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO payment_attempts (payment_id, attempt_no, status, provider_reference, detail)
			VALUES ($1, COALESCE((SELECT MAX(attempt_no)+1 FROM payment_attempts WHERE payment_id=$1),1),
				'failed', $2, '{"simulated":true,"via":"webhook"}'::jsonb)`,
			pay.id, evt.ProviderPayRef); err != nil {
			httpx.WriteError(w, r, err)
			return
		}

	default:
		_, _ = tx.Exec(r.Context(), `
			UPDATE payment_webhook_events SET processing_error = 'unknown event type'
			WHERE provider='mock' AND event_id = $1`, evt.EventID)
		_ = tx.Commit(r.Context())
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Unknown webhook event type.", nil)
		return
	}

	// mark webhook processed
	if _, err = tx.Exec(r.Context(),
		`UPDATE payment_webhook_events SET processed_at = now() WHERE provider='mock' AND event_id = $1`,
		evt.EventID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	var newStatus string
	_ = h.pool.QueryRow(r.Context(), `SELECT status FROM payments WHERE id = $1`, pay.id).Scan(&newStatus)
	slog.InfoContext(r.Context(), "webhook processed",
		slog.String("event_id", evt.EventID), slog.String("type", evt.Type), slog.String("payment", pay.id))
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"processed": true, "status": newStatus})
}

// transitionOrder performs a guarded state transition inside a transaction.
// The WHERE state = expected clause makes concurrent transitions safe.
func transitionOrder(ctx context.Context, q db.Querier, orderID, from, to, actor, reason string) error {
	tag, err := q.Exec(ctx, `
		UPDATE orders SET status = $3, updated_at = now(),
			placed_at = CASE WHEN $3 = 'placed' THEN now() ELSE placed_at END
		WHERE id = $1 AND status = $2`, orderID, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.New(apperr.CodeInvalidState,
			fmt.Sprintf("Order is not in state %s anymore.", from))
	}
	_, err = q.Exec(ctx, `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, reason)
		VALUES ($1, 'state_changed', $2, $3, $4, $5)`, orderID, from, to, actor, reason)
	return err
}

// --- refunds --------------------------------------------------------------

type refundBody struct {
	Reason string `json:"reason"`
}

// RequestRefund creates a refund for a captured online payment. The mock
// provider completes it immediately; production adapters would leave it
// pending until provider confirmation. Policy: allowed while the order is
// rejected/cancelled/delivery_failed (auto flows already refund) or
// delivered within the demo support window.
func (h *Handlers) RequestRefund(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	orderID := r.PathValue("id")
	var body refundBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if len(body.Reason) < 4 || len(body.Reason) > 500 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Please describe the refund reason (4–500 characters).", nil)
		return
	}

	// ownership + refundable state
	var pay struct {
		id       string
		provRef  *string
		status   string
		method   string
		amount   int64
		orderRef string
	}
	err := h.pool.QueryRow(r.Context(), `
		SELECT pm.id, pm.provider_reference, pm.status, pm.method, pm.amount_paise, o.reference
		FROM payments pm JOIN orders o ON o.id = pm.order_id
		WHERE pm.order_id = $1 AND o.customer_id = $2
		ORDER BY pm.created_at DESC LIMIT 1`, orderID, p.ID).Scan(
		&pay.id, &pay.provRef, &pay.status, &pay.method, &pay.amount, &pay.orderRef)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Order or payment not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if pay.method == "cod" {
		httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
			"Cash-on-delivery orders are not refunded online.", nil)
		return
	}
	if pay.status != "captured" {
		httpx.Fail(w, r, http.StatusConflict, "invalid_state_transition",
			fmt.Sprintf("Payment is %s — nothing to refund.", pay.status), nil)
		return
	}

	// provider refund (mock: instant success) + records
	providerRef, err := h.provider.Refund(r.Context(), deref(pay.provRef), pay.amount, body.Reason)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO refunds (payment_id, order_id, amount_paise, status, reason, provider_reference, initiated_by, completed_at)
		VALUES ($1,$2,$3,'processed',$4,$5,'customer',now())`,
		pay.id, orderID, pay.amount, body.Reason, providerRef); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		UPDATE payments SET status='refunded', updated_at=now() WHERE id=$1`, pay.id); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id, reason)
		VALUES ($1,'refund_processed',NULL,NULL,'customer',$2,$3)`, orderID, p.ID, body.Reason); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = outbox.Enqueue(r.Context(), tx, "payment", pay.id, "payment.refunded", map[string]any{
		"order_id": orderID, "payment_id": pay.id, "amount_paise": pay.amount,
	}, ptrStr("refund:"+pay.id)); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, map[string]any{
		"refund_status": "processed",
		"simulated":     true,
		"demo_note":     "Refund completed by the local mock provider. Production requires provider confirmation.",
	})
}

// --- helpers --------------------------------------------------------------

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrStr(s string) *string { return &s }

func baseURLFrom(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8080"
	}
	return scheme + "://" + host
}

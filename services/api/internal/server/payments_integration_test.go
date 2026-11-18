package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
	"time"

	mockpay "biteorbit/services/api/internal/platform/payments"
)

// setupOnlineOrder creates a customer + address + cart + quote + placement
// with a simulated online payment. Returns (jar, orderID, paymentID, reference).
func setupOnlineOrder(t *testing.T, email, method string) (jar *cookiejar.Jar, orderID, paymentID, reference string) {
	t.Helper()
	jar, addressID, itemID := setupCheckout(t, email)
	_, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemID, "quantity": 2,
	})
	status, res := env.Do(t, jar, "POST", "/api/v1/checkout/quote", map[string]any{
		"address_id": addressID, "payment_method": method,
	})
	if status != http.StatusOK {
		t.Fatalf("quote = %d (%v)", status, res.Error)
	}
	var q struct {
		QuoteID string `json:"quote_id"`
	}
	_ = json.Unmarshal(res.Data, &q)
	status, res = env.DoWithHeader(t, jar, "POST", "/api/v1/checkout/place",
		map[string]any{"quote_id": q.QuoteID},
		map[string]string{"Idempotency-Key": "setup-" + email})
	if status != http.StatusOK {
		t.Fatalf("place = %d (%v)", status, res.Error)
	}
	var p struct {
		OrderID   string `json:"order_id"`
		PaymentID string `json:"payment_id"`
		Reference string `json:"reference"`
		Status    string `json:"status"`
	}
	_ = json.Unmarshal(res.Data, &p)
	if p.Status != "pending_payment" {
		t.Fatalf("online order status = %s, want pending_payment", p.Status)
	}
	return jar, p.OrderID, p.PaymentID, p.Reference
}

func TestPaymentCaptureFlow(t *testing.T) {
	email := "pay-capture@example.test"
	jar, orderID, paymentID, _ := setupOnlineOrder(t, email, "mock_upi")

	status, res := env.Do(t, jar, "GET", "/api/v1/orders/"+orderID, nil)
	var detail map[string]any
	_ = json.Unmarshal(res.Data, &detail)
	if detail["status"] != "pending_payment" {
		t.Fatalf("order status = %v", detail["status"])
	}

	status, res = env.Do(t, jar, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})
	if status != http.StatusOK {
		t.Fatalf("simulate = %d (%v)", status, res.Error)
	}
	var sim struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(res.Data, &sim)
	if sim.Status != "captured" {
		t.Fatalf("payment status = %s, want captured", sim.Status)
	}

	status, res = env.Do(t, jar, "GET", "/api/v1/orders/"+orderID, nil)
	_ = json.Unmarshal(res.Data, &detail)
	if detail["status"] != "placed" {
		t.Fatalf("order after capture = %v, want placed", detail["status"])
	}

	var events int
	_ = env.Pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM order_events WHERE order_id = $1 AND to_state = 'placed'`, orderID).Scan(&events)
	if events < 1 {
		t.Fatal("no placed event recorded")
	}
}

func TestPaymentFailureAndRetry(t *testing.T) {
	email := "pay-failure@example.test"
	jar, orderID, paymentID, _ := setupOnlineOrder(t, email, "mock_card")

	status, res := env.Do(t, jar, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "failure"})
	if status != http.StatusOK {
		t.Fatalf("simulate failure = %d", status)
	}
	var sim struct {
		Status string `json:"status"`
	}
	_ = json.Unmarshal(res.Data, &sim)
	if sim.Status != "failed" {
		t.Fatalf("status = %s, want failed", sim.Status)
	}

	status, res = env.Do(t, jar, "GET", "/api/v1/orders/"+orderID, nil)
	var detail map[string]any
	_ = json.Unmarshal(res.Data, &detail)
	if detail["status"] != "pending_payment" {
		t.Fatalf("order status after failed payment = %v", detail["status"])
	}

	status, res = env.Do(t, jar, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})
	_ = json.Unmarshal(res.Data, &sim)
	if status != http.StatusOK || sim.Status != "captured" {
		t.Fatalf("retry = %d %s", status, sim.Status)
	}

	var attempts int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM payment_attempts WHERE payment_id = $1`, paymentID).Scan(&attempts)
	if attempts < 2 {
		t.Fatalf("attempts = %d, want >=2", attempts)
	}
}

func TestWebhookSecurityPaths(t *testing.T) {
	_, _, paymentID, reference := setupOnlineOrder(t, "pay-webhook@example.test", "mock_upi")

	var provRef string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT provider_reference FROM payments WHERE id = $1`, paymentID).Scan(&provRef)
	var amount int64
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT amount_paise FROM payments WHERE provider_reference = $1`, provRef).Scan(&amount)

	// valid signature over a TAMPERED amount: amount check must reject
	tampered, _ := json.Marshal(map[string]any{
		"event_id": "evt_evil_1", "type": "payment.captured",
		"provider_payment_ref": provRef, "order_reference": reference,
		"amount_paise": 1, "timestamp": time.Now().Unix(),
	})
	if code := postRawWebhook(t, tampered, mockpay.SignMockWebhook(tampered)); code != http.StatusBadRequest {
		t.Fatalf("tampered amount webhook = %d, want 400", code)
	}

	// bad signature → 400 and audited rejection
	if code := postRawWebhook(t, []byte(`{"event_id":"evt_bad"}`), "deadbeef"); code != http.StatusBadRequest {
		t.Fatalf("bad signature = %d, want 400", code)
	}
	var invalid int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM payment_webhook_events WHERE signature_valid = false`).Scan(&invalid)
	if invalid < 1 {
		t.Fatal("rejected webhook not audited")
	}

	// legitimate capture for the replay test
	okBody, _ := json.Marshal(map[string]any{
		"event_id": "evt_replay_1", "type": "payment.captured",
		"provider_payment_ref": provRef, "order_reference": reference,
		"amount_paise": amount, "timestamp": time.Now().Unix(),
	})
	if code := postRawWebhook(t, okBody, mockpay.SignMockWebhook(okBody)); code != http.StatusOK {
		t.Fatalf("valid webhook = %d", code)
	}

	// replay the SAME event id → deduplicated, no double transition
	if code := postRawWebhook(t, okBody, mockpay.SignMockWebhook(okBody)); code != http.StatusOK {
		t.Fatalf("replay webhook = %d", code)
	}
	var status string
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&status)
	if status != "captured" {
		t.Fatalf("payment status after replay = %s", status)
	}
	var processed int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM payment_webhook_events WHERE event_id = 'evt_replay_1' AND processed_at IS NOT NULL`).Scan(&processed)
	if processed != 1 {
		t.Fatalf("webhook rows = %d, want 1 (deduplicated)", processed)
	}
}

// postRawWebhook posts a raw body to the webhook endpoint with a signature.
func postRawWebhook(t *testing.T, body []byte, signature string) int {
	t.Helper()
	req, _ := http.NewRequest("POST", env.URL+"/api/v1/webhooks/mock", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-BiteOrbit-Signature", signature)
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return -1
	}
	defer res.Body.Close()
	return res.StatusCode
}

func TestCustomerCancelFlow(t *testing.T) {
	// COD order placed → cancelled by customer with event trail
	email := "cancel-cod@example.test"
	jar, orderID, _, _ := setupOnlineOrder(t, email, "mock_upi")
	// capture the payment first so the refund path is exercised
	_, res := env.Do(t, jar, "GET", "/api/v1/orders/"+orderID, nil)
	var detail map[string]any
	_ = json.Unmarshal(res.Data, &detail)
	payment := detail["payment"].(map[string]any)["payment"].(map[string]any)
	paymentID := payment["id"].(string)
	_, _ = env.Do(t, jar, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})

	status, res := env.Do(t, jar, "POST", "/api/v1/orders/"+orderID+"/cancel", map[string]any{
		"reason": "Made a duplicate order by mistake",
	})
	if status != http.StatusOK {
		t.Fatalf("cancel = %d (%v)", status, res.Error)
	}
	var result struct {
		Status          string `json:"status"`
		RefundInitiated bool   `json:"refund_initiated"`
	}
	_ = json.Unmarshal(res.Data, &result)
	if result.Status != "cancelled" {
		t.Fatalf("cancel status = %s", result.Status)
	}
	if !result.RefundInitiated {
		t.Fatal("captured online payment must be refunded on cancellation")
	}

	// refund record exists and payment refunded
	var refunded int
	_ = env.Pool.QueryRow(t.Context(),
		`SELECT COUNT(*) FROM refunds WHERE order_id = $1 AND status='processed'`, orderID).Scan(&refunded)
	if refunded != 1 {
		t.Fatalf("refund rows = %d", refunded)
	}

	// cancelling again must be rejected (terminal state)
	status, res = env.Do(t, jar, "POST", "/api/v1/orders/"+orderID+"/cancel", map[string]any{"reason": "again"})
	if status != http.StatusConflict {
		t.Fatalf("second cancel = %d, want 409", status)
	}
}

func TestCustomerCancelAfterPickupRejected(t *testing.T) {
	email := "cancel-late@example.test"
	jar, orderID, _, _ := setupOnlineOrder(t, email, "mock_upi")
	// move the order to a post-pickup state directly (simulating fulfilment)
	_, _ = env.Pool.Exec(t.Context(),
		`UPDATE orders SET status='picked_up', picked_at=now() WHERE id=$1`, orderID)
	status, res := env.Do(t, jar, "POST", "/api/v1/orders/"+orderID+"/cancel", map[string]any{
		"reason": "too late",
	})
	if status != http.StatusConflict {
		t.Fatalf("late cancel = %d, want 409 (%v)", status, res.Error)
	}
}

func TestOrderHistoryAndIDOR(t *testing.T) {
	email := "history-owner@example.test"
	jar, orderID, _, reference := setupOnlineOrder(t, email, "mock_upi")

	// history lists the order
	status, res := env.Do(t, jar, "GET", "/api/v1/orders", nil)
	if status != http.StatusOK {
		t.Fatalf("history = %d", status)
	}
	var list []map[string]any
	_ = json.Unmarshal(res.Data, &list)
	found := false
	for _, o := range list {
		if o["reference"] == reference {
			found = true
		}
	}
	if !found {
		t.Fatal("order missing from history")
	}

	// detail has timeline + items + breakdown
	status, res = env.Do(t, jar, "GET", "/api/v1/orders/"+orderID, nil)
	var detail struct {
		Reference string `json:"reference"`
		Events    []any  `json:"events"`
		Items     []any  `json:"items"`
		Breakdown map[string]any
	}
	_ = json.Unmarshal(res.Data, &detail)
	if detail.Reference != reference || len(detail.Events) == 0 || len(detail.Items) == 0 {
		t.Fatalf("order detail incomplete: %v", detail)
	}

	// IDOR: another user cannot see the order
	env.RegisterCustomer(t, "history-thief@example.test")
	thiefJar := env.NewJar()
	_, _ = env.Do(t, thiefJar, "POST", "/api/v1/auth/login", map[string]any{
		"email": "history-thief@example.test", "password": "Passw0rdLocal!",
	})
	status, _ = env.Do(t, thiefJar, "GET", "/api/v1/orders/"+orderID, nil)
	if status != http.StatusNotFound {
		t.Fatalf("thief order detail = %d, want 404", status)
	}
	status, _ = env.Do(t, thiefJar, "POST", "/api/v1/orders/"+orderID+"/cancel", map[string]any{"reason": "sabotage"})
	if status != http.StatusNotFound {
		t.Fatalf("thief cancel = %d, want 404", status)
	}
}

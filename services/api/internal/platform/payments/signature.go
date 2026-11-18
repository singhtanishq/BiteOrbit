package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// The mock provider's webhook channel uses HMAC-SHA256 over the raw body with
// a shared demo secret. This exercises exactly the same verification path a
// real gateway integration would use (signature + timestamp + replay guard).
const mockWebhookSecret = "biteorbit-local-mock-webhook-secret"

// SignMockWebhook produces a valid signature for the local mock provider.
// Only the local demo payment flow uses this; it is not a real secret.
func SignMockWebhook(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(mockWebhookSecret))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

type mockWebhookBody struct {
	EventID        string `json:"event_id"`
	Type           string `json:"type"`
	ProviderPayRef string `json:"provider_payment_ref"`
	OrderReference string `json:"order_reference"`
	AmountPaise    int64  `json:"amount_paise"`
	Timestamp      int64  `json:"timestamp"`
}

// verifyMockSignature checks the HMAC and a 10-minute timestamp window
// (replay protection) and decodes the event.
func verifyMockSignature(payload []byte, signature string) (*WebhookEvent, bool, error) {
	mac := hmac.New(sha256.New, []byte(mockWebhookSecret))
	mac.Write(payload)
	expected := mac.Sum(nil)
	got, err := hex.DecodeString(signature)
	if err != nil || !hmac.Equal(got, expected) {
		return nil, false, nil
	}
	var body mockWebhookBody
	if err := json.Unmarshal(payload, &body); err != nil {
		return nil, false, fmt.Errorf("decode webhook body: %w", err)
	}
	// Replay protection: reject events older than 10 minutes.
	if time.Since(time.Unix(body.Timestamp, 0)) > 10*time.Minute {
		return nil, false, fmt.Errorf("webhook timestamp outside acceptance window")
	}
	return &WebhookEvent{
		EventID:        body.EventID,
		Type:           body.Type,
		ProviderPayRef: body.ProviderPayRef,
		OrderReference: body.OrderReference,
		AmountPaise:    body.AmountPaise,
		SignatureValid: true,
	}, true, nil
}

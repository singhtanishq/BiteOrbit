// Package payments defines the payment provider interface and the local mock
// adapter. The mock is clearly labelled simulated everywhere it appears; it
// never collects real money and never stores card data.
package payments

import (
	"context"
	"fmt"
)

// InitRequest starts a payment for an order.
type InitRequest struct {
	OrderID     string
	Reference   string
	AmountPaise int64
	Method      string // mock_upi | mock_card | mock_netbanking | cod
	Currency    string
}

// InitResult describes what the client should render.
type InitResult struct {
	ProviderPaymentID string
	NextAction        string // "confirm" | "none" (COD)
	Simulated         bool
}

// WebhookEvent is a verified provider notification.
type WebhookEvent struct {
	EventID        string
	Type           string // payment.captured | payment.failed | refund.processed
	ProviderPayRef string
	OrderReference string
	AmountPaise    int64
	SignatureValid bool
}

// Provider abstracts a payment gateway.
type Provider interface {
	Name() string
	Init(ctx context.Context, req InitRequest) (*InitResult, error)
	// VerifyWebhook validates the signature and decodes the event; ok=false
	// means the caller must reject the webhook.
	VerifyWebhook(payload []byte, signature string) (*WebhookEvent, bool, error)
	// Refund requests a (partial) refund; production adapters require provider
	// confirmation before status becomes processed.
	Refund(ctx context.Context, providerPaymentID string, amountPaise int64, reason string) (providerRef string, err error)
}

// MockProvider is the local simulated gateway. Success/failure is decided by
// the caller via demo payment UI calls; webhook-style notifications are
// produced through the same signed channel so the server verification path
// is identical to a production provider.
type MockProvider struct{}

func NewMock() *MockProvider { return &MockProvider{} }

func (m *MockProvider) Name() string { return "mock" }

func (m *MockProvider) Init(ctx context.Context, req InitRequest) (*InitResult, error) {
	if req.Method == "cod" {
		return &InitResult{NextAction: "none", Simulated: true}, nil
	}
	id := fmt.Sprintf("mockpay_%s", req.Reference)
	return &InitResult{ProviderPaymentID: id, NextAction: "confirm", Simulated: true}, nil
}

// VerifyWebhook delegates to the HMAC verification shared with the demo flow.
func (m *MockProvider) VerifyWebhook(payload []byte, signature string) (*WebhookEvent, bool, error) {
	return verifyMockSignature(payload, signature)
}

func (m *MockProvider) Refund(ctx context.Context, providerPaymentID string, amountPaise int64, reason string) (string, error) {
	// The mock always "succeeds" immediately; real adapters may return pending.
	return "mockrefund_" + providerPaymentID, nil
}

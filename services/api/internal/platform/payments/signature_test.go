package payments

import (
	"encoding/json"
	"testing"
	"time"
)

func TestWebhookSignatureRoundTrip(t *testing.T) {
	body, _ := json.Marshal(mockWebhookBody{
		EventID:        "evt_1",
		Type:           "payment.captured",
		ProviderPayRef: "mockpay_X",
		OrderReference: "BO-00001",
		AmountPaise:    45000,
		Timestamp:      time.Now().Unix(),
	})
	sig := SignMockWebhook(body)

	evt, ok, err := verifyMockSignature(body, sig)
	if err != nil || !ok {
		t.Fatalf("valid signature must verify (ok=%v err=%v)", ok, err)
	}
	if evt.Type != "payment.captured" || evt.AmountPaise != 45000 {
		t.Fatalf("event decoded incorrectly: %+v", evt)
	}
}

func TestWebhookSignatureRejected(t *testing.T) {
	body, _ := json.Marshal(mockWebhookBody{
		EventID: "evt_2", Type: "payment.captured", ProviderPayRef: "mockpay_X",
		OrderReference: "BO-00001", AmountPaise: 100, Timestamp: time.Now().Unix(),
	})
	if _, ok, _ := verifyMockSignature(body, "deadbeef"); ok {
		t.Fatal("bad signature must be rejected")
	}
	if _, ok, _ := verifyMockSignature([]byte(`{"tampered":true}`), SignMockWebhook(body)); ok {
		t.Fatal("signature over different body must be rejected")
	}
}

func TestWebhookReplayRejected(t *testing.T) {
	old := time.Now().Add(-15 * time.Minute).Unix()
	body, _ := json.Marshal(mockWebhookBody{
		EventID: "evt_3", Type: "payment.captured", ProviderPayRef: "mockpay_X",
		OrderReference: "BO-00001", AmountPaise: 100, Timestamp: old,
	})
	if _, ok, _ := verifyMockSignature(body, SignMockWebhook(body)); ok {
		t.Fatal("stale webhook (replay) must be rejected")
	}
}

func TestMockInitIsSimulated(t *testing.T) {
	p := NewMock()
	res, err := p.Init(t.Context(), InitRequest{Reference: "BO-1", AmountPaise: 100, Method: "mock_upi"})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !res.Simulated {
		t.Fatal("mock provider must flag results as simulated")
	}
	if res.NextAction != "confirm" {
		t.Fatalf("online method should require confirm, got %q", res.NextAction)
	}
	cod, _ := p.Init(t.Context(), InitRequest{Method: "cod"})
	if cod.NextAction != "none" {
		t.Fatal("COD should not require confirmation")
	}
}

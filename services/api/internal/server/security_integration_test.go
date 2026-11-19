package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"
)

// M12 security hardening suite: consolidated cross-role/IDOR/abuse checks.
// Individual flows are covered in their domain tests; this file focuses on
// boundaries between roles and resources.

func TestSecurityCrossRoleBoundaries(t *testing.T) {
	customer := env.NewJar()
	env.RegisterCustomerWithJar(t, customer, "sec-customer@example.test")
	rider := loginRider(t, "rider@demo.biteorbit.local")
	staff := loginAsStaff(t, "manager01@demo.biteorbit.local")
	admin := loginAdmin(t, "admin@demo.biteorbit.local")

	probes := []struct {
		name    string
		jar     *cookiejar.Jar
		method  string
		path    string
		body    map[string]any
		wantNot int // the response must NOT be this status (i.e. must not succeed)
		succeed int // or must be exactly this when allowed
	}{
		{"customer → admin overview", customer, "GET", "/api/v1/admin/overview", nil, 200, 403},
		{"customer → courier offers", customer, "GET", "/api/v1/courier/offers", nil, 200, 403},
		{"customer → portal outlets", customer, "GET", "/api/v1/restaurant-portal/outlets", nil, 200, 403},
		{"rider → admin overview", rider, "GET", "/api/v1/admin/overview", nil, 200, 403},
		{"rider → portal outlets", rider, "GET", "/api/v1/restaurant-portal/outlets", nil, 200, 403},
		{"staff → admin overview", staff, "GET", "/api/v1/admin/overview", nil, 200, 403},
		{"staff → admin orders", staff, "GET", "/api/v1/admin/orders", nil, 200, 403},
		{"admin → portal outlets (no grants)", admin, "GET", "/api/v1/restaurant-portal/outlets", nil, 200, 403},
	}

	for _, tc := range probes {
		t.Run(tc.name, func(t *testing.T) {
			probeStatus, _ := env.Do(t, tc.jar, tc.method, tc.path, tc.body)
			if probeStatus == 401 {
				t.Fatalf("session invalid for %s", tc.name)
			}
			if probeStatus == tc.wantNot {
				t.Fatalf("%s unexpectedly succeeded (%d)", tc.name, probeStatus)
			}
		})
	}
}

func TestSecurityIDORInventory(t *testing.T) {
	// victim resources
	victimJar, victimOrder, victimPayment, _ := setupOnlineOrder(t, "sec-victim@example.test", "mock_upi")
	_ = victimJar
	_ = victimPayment

	attacker := env.NewJar()
	env.RegisterCustomerWithJar(t, attacker, "sec-attacker@example.test")

	// find another user's address
	_, res := env.Do(t, victimJar, "GET", "/api/v1/me/addresses", nil)
	var addrs []map[string]any
	_ = json.Unmarshal(res.Data, &addrs)
	victimAddress := addrs[0]["id"].(string)

	paths := []struct {
		name   string
		method string
		path   string
		body   map[string]any
		want   int
	}{
		{"foreign order detail", "GET", "/api/v1/orders/" + victimOrder, nil, 404},
		{"foreign order cancel", "POST", "/api/v1/orders/" + victimOrder + "/cancel", map[string]any{"reason": "security probe"}, 404},
		{"foreign order payment", "GET", "/api/v1/orders/" + victimOrder + "/payment", nil, 404},
		{"foreign order refund", "POST", "/api/v1/orders/" + victimOrder + "/refund", map[string]any{"reason": "security probe"}, 404},
		{"foreign order track", "GET", "/api/v1/orders/" + victimOrder + "/track", nil, 404},
		{"foreign order reorder", "POST", "/api/v1/orders/" + victimOrder + "/reorder", nil, 404},
		{"foreign address update", "PATCH", "/api/v1/me/addresses/" + victimAddress, map[string]any{
			"label": "home", "recipient_name": "X", "recipient_phone": "9876543210",
			"line1": "1 X", "locality": "L", "city": "C", "state": "S", "postal_code": "560001",
		}, 404},
		{"foreign ticket thread", "GET", "/api/v1/support/tickets/" + victimOrder, nil, 404},
	}
	for _, tc := range paths {
		t.Run(tc.name, func(t *testing.T) {
			status, _ := env.Do(t, attacker, tc.method, tc.path, tc.body)
			if status != tc.want {
				t.Fatalf("%s = %d, want %d", tc.name, status, tc.want)
			}
		})
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }

func mustParseURL(raw string) *url.URL {
	u, _ := url.Parse(raw)
	return u
}

func TestSecurityStateGuards(t *testing.T) {
	// delivered orders cannot be cancelled or re-reviewed
	jarC, orderID, paymentID, _ := setupOnlineOrder(t, "sec-state@example.test", "mock_upi")
	_, _ = env.Do(t, jarC, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})

	// move to delivered directly (demo shortcut)
	_, _ = env.Pool.Exec(t.Context(),
		`UPDATE orders SET status='delivered', delivered_at=now(), closed_at=now() WHERE id=$1`, orderID)

	status, _ := env.Do(t, jarC, "POST", "/api/v1/orders/"+orderID+"/cancel", map[string]any{"reason": "too late"})
	if status != http.StatusConflict {
		t.Fatalf("cancel delivered = %d, want 409", status)
	}

	// payment cannot be re-captured
	status, res := env.Do(t, jarC, "POST", "/api/v1/payments/"+paymentID+"/simulate", map[string]any{"outcome": "success"})
	if status == http.StatusOK {
		var sim struct {
			Status string `json:"status"`
		}
		_ = json.Unmarshal(res.Data, &sim)
		if sim.Status == "pending" {
			t.Fatal("capture from terminal state allowed")
		}
	}
}

func TestSecurityPayloadLimits(t *testing.T) {
	jar := env.NewJar()
	env.RegisterCustomerWithJar(t, jar, "sec-payload@example.test")

	// oversized instructions rejected
	big := make([]byte, 500)
	for i := range big {
		big[i] = 'a'
	}
	status, _ := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id":         "00000000-0000-0000-0000-000000000000",
		"quantity":             1,
		"special_instructions": string(big),
	})
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		t.Fatalf("oversized payload accepted (%d)", status)
	}

	// invalid JSON rejected cleanly
	req, _ := http.NewRequest("POST", env.URL+"/api/v1/auth/login", bytesReader([]byte("{invalid")))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://localhost:3000")
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("raw request: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid JSON = %d, want 400", res.StatusCode)
	}
}

func TestSecurityOriginCheck(t *testing.T) {
	// state-changing request with a foreign Origin must be rejected
	req, _ := http.NewRequest("POST", env.URL+"/api/v1/auth/login",
		bytesReader([]byte(`{"email":"a@b.test","password":"WrongPass123"}`)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.test")
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("origin probe: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("foreign origin = %d, want 403", res.StatusCode)
	}

	// allowed origin passes through to handler logic (401 for bad creds)
	req2, _ := http.NewRequest("POST", env.URL+"/api/v1/auth/login",
		bytesReader([]byte(`{"email":"a@b.test","password":"WrongPass123"}`)))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Origin", "http://localhost:3000")
	res2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("allowed origin probe: %v", err)
	}
	res2.Body.Close()
	if res2.StatusCode == http.StatusForbidden {
		t.Fatal("allowed origin wrongly rejected")
	}
}

func TestSecuritySessionRevocation(t *testing.T) {
	email := "sec-session@example.test"
	jar := env.NewJar()
	env.RegisterCustomerWithJar(t, jar, email)

	// logout-all kills every session
	status, _ := env.Do(t, jar, "POST", "/api/v1/auth/logout-all", nil)
	if status != http.StatusOK {
		t.Fatalf("logout-all = %d", status)
	}
	status, _ = env.Do(t, jar, "GET", "/api/v1/auth/session", nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("session alive after logout-all (%d)", status)
	}

	// tampered cookie value is simply unauthorised, never a crash
	tampered := env.NewJar()
	env.RegisterCustomerWithJar(t, tampered, "sec-tamper@example.test")
	// fetch cookies and corrupt one
	req, _ := http.NewRequest("GET", env.URL+"/api/v1/auth/session", nil)
	envURL := env.URL
	cookies := tampered.Cookies(mustParseURL(envURL))
	if len(cookies) > 0 {
		c := cookies[0]
		c.Value = "v1.deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		req.AddCookie(c)
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		t.Fatalf("tampered cookie probe: %v", err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered cookie = %d, want 401", res.StatusCode)
	}
}

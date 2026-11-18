package server_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"biteorbit/services/api/internal/auth"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/itest"
)

var env *itest.Env

func TestMain(m *testing.M) {
	env = itest.MainStart()
	// Deterministic demo dataset for the whole package run.
	if err := env.MustSeed(); err != nil {
		slog.Error("test seed failed", slog.Any("err", err))
		os.Exit(1)
	}
	code := m.Run()
	env.Teardown()
	os.Exit(code)
}

func TestRegisterLoginSessionFlow(t *testing.T) {
	email := "flow-user@example.test"
	jar := env.NewJar()

	// register
	status, res := env.Do(t, jar, "POST", "/api/v1/auth/register", map[string]any{
		"email": email, "password": "Passw0rdLocal!", "name": "Flow User", "phone": "+919876543210",
	})
	if status != http.StatusCreated {
		t.Fatalf("register status = %d, want 201", status)
	}
	var user struct {
		ID    string   `json:"id"`
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(res.Data, &user); err != nil {
		t.Fatalf("register response: %v", err)
	}
	if len(user.Roles) != 1 || user.Roles[0] != "customer" {
		t.Fatalf("expected customer role, got %v", user.Roles)
	}

	// session follows from the cookie
	status, res = env.Do(t, jar, "GET", "/api/v1/auth/session", nil)
	if status != http.StatusOK {
		t.Fatalf("session status = %d", status)
	}
	var session struct {
		Email string `json:"email"`
	}
	_ = json.Unmarshal(res.Data, &session)
	if session.Email != email {
		t.Fatalf("session email = %q", session.Email)
	}

	// duplicate registration is rejected
	status, res = env.Do(t, env.NewJar(), "POST", "/api/v1/auth/register", map[string]any{
		"email": email, "password": "Passw0rdLocal!", "name": "Dup",
	})
	if status != http.StatusConflict {
		t.Fatalf("duplicate register status = %d, want 409", status)
	}

	// refresh rotates the session: old token must die
	oldJar := env.NewJar()
	_, _ = env.Do(t, oldJar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rdLocal!",
	})
	status, _ = env.Do(t, oldJar, "POST", "/api/v1/auth/refresh", nil)
	if status != http.StatusOK {
		t.Fatalf("refresh status = %d", status)
	}
	// new cookie works
	if status, _ := env.Do(t, oldJar, "GET", "/api/v1/auth/session", nil); status != http.StatusOK {
		t.Fatalf("session after refresh = %d, want 200", status)
	}

	// logout revokes
	status, _ = env.Do(t, oldJar, "POST", "/api/v1/auth/logout", nil)
	if status != http.StatusOK {
		t.Fatalf("logout status = %d", status)
	}
	if status, _ := env.Do(t, oldJar, "GET", "/api/v1/auth/session", nil); status != http.StatusUnauthorized {
		t.Fatalf("session after logout = %d, want 401", status)
	}
}

func TestLoginIsEnumerationSafe(t *testing.T) {
	wrongPass, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/login", map[string]any{
		"email": "known-user@example.test", "password": "WrongPass123",
	})
	unknownEmail, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/login", map[string]any{
		"email": "definitely-not-registered@example.test", "password": "WrongPass123",
	})
	if wrongPass != unknownEmail {
		t.Fatalf("login responses must be identical for unknown email vs wrong password (%d vs %d)", wrongPass, unknownEmail)
	}
	if wrongPass != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", wrongPass)
	}
}

func TestPasswordResetFlow(t *testing.T) {
	email := "reset-user@example.test"
	env.RegisterCustomer(t, email)

	// request reset
	status, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/password-reset", map[string]any{"email": email})
	if status != http.StatusAccepted {
		t.Fatalf("reset request status = %d", status)
	}
	// unknown email gets the identical response (enumeration safety)
	status2, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/password-reset", map[string]any{
		"email": "no-such-user@example.test",
	})
	if status2 != status {
		t.Fatalf("reset responses differ (%d vs %d)", status, status2)
	}

	// extract token from captured email
	if len(env.Email.Messages) == 0 {
		t.Fatal("expected a reset email to be captured")
	}
	last := env.Email.Messages[len(env.Email.Messages)-1]
	if last.To != email {
		t.Fatalf("reset email sent to %q, want %q", last.To, email)
	}
	token := extractToken(last.Body)

	// confirm with a weak password fails
	status, _ = env.Do(t, env.NewJar(), "POST", "/api/v1/auth/password-reset/confirm", map[string]any{
		"token": token, "new_password": "weak",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("weak password confirm = %d", status)
	}

	// confirm properly
	status, _ = env.Do(t, env.NewJar(), "POST", "/api/v1/auth/password-reset/confirm", map[string]any{
		"token": token, "new_password": "NewPassw0rd456!",
	})
	if status != http.StatusOK {
		t.Fatalf("confirm status = %d", status)
	}

	// old password no longer works; new one does
	if s, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rdLocal!",
	}); s != http.StatusUnauthorized {
		t.Fatalf("old password still accepted (%d)", s)
	}
	if s, _ := env.Do(t, env.NewJar(), "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "NewPassw0rd456!",
	}); s != http.StatusOK {
		t.Fatalf("new password rejected (%d)", s)
	}

	// token cannot be reused
	status, _ = env.Do(t, env.NewJar(), "POST", "/api/v1/auth/password-reset/confirm", map[string]any{
		"token": token, "new_password": "AnotherPass0rd!",
	})
	if status != http.StatusBadRequest {
		t.Fatalf("token reuse = %d, want 400", status)
	}
}

func extractToken(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if len(line) == 48 && isHex(line) {
			return line
		}
	}
	return ""
}

func isHex(s string) bool {
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return false
		}
	}
	return true
}

func TestAddressOwnershipIDOR(t *testing.T) {
	owner := "addr-owner@example.test"
	attacker := "addr-attacker@example.test"

	ownerJar := env.NewJar()
	_, _ = env.Do(t, ownerJar, "POST", "/api/v1/auth/register", map[string]any{
		"email": owner, "password": "Passw0rdLocal!", "name": "Owner",
	})
	status, res := env.Do(t, ownerJar, "POST", "/api/v1/me/addresses", map[string]any{
		"label": "home", "recipient_name": "Owner", "recipient_phone": "9876543210",
		"line1": "1 Rosewood Lane", "locality": "Rosewood Hills", "city": "Bitepur",
		"state": "Telengana Heights", "postal_code": "560001",
	})
	if status != http.StatusCreated {
		t.Fatalf("create address = %d (%s)", status, res.Error.Message)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(res.Data, &created)

	// attacker cannot see/modify owner's address
	attackerJar := env.NewJar()
	_, _ = env.Do(t, attackerJar, "POST", "/api/v1/auth/register", map[string]any{
		"email": attacker, "password": "Passw0rdLocal!", "name": "Attacker",
	})

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"read", "GET", "/api/v1/me/addresses", nil},
		{"update", "PATCH", "/api/v1/me/addresses/" + created.ID, map[string]any{
			"label": "home", "recipient_name": "Hacked", "recipient_phone": "9876543210",
			"line1": "1 Rosewood Lane", "locality": "Rosewood Hills", "city": "Bitepur",
			"state": "Telengana Heights", "postal_code": "560001",
		}},
		{"delete", "DELETE", "/api/v1/me/addresses/" + created.ID, nil},
		{"default", "POST", "/api/v1/me/addresses/" + created.ID + "/default", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.path
			if tc.name == "read" {
				// list is scoped: owner's address must not appear for attacker
				status, res := env.Do(t, attackerJar, "GET", path, nil)
				if status != http.StatusOK {
					t.Fatalf("list status = %d", status)
				}
				var addrs []map[string]any
				_ = json.Unmarshal(res.Data, &addrs)
				for _, a := range addrs {
					if a["id"] == created.ID {
						t.Fatal("attacker can see another user's address (IDOR)")
					}
				}
				return
			}
			status, _ := env.Do(t, attackerJar, tc.method, path, tc.body)
			if status != http.StatusNotFound {
				t.Fatalf("attacker %s on foreign address = %d, want 404", tc.name, status)
			}
		})
	}

	// owner can still access it
	status, _ = env.Do(t, ownerJar, "POST", "/api/v1/me/addresses/"+created.ID+"/default", nil)
	if status != http.StatusOK {
		t.Fatalf("owner set default = %d", status)
	}
}

func TestAddressValidation(t *testing.T) {
	jar := env.NewJar()
	_, _ = env.Do(t, jar, "POST", "/api/v1/auth/register", map[string]any{
		"email": "addr-validate@example.test", "password": "Passw0rdLocal!", "name": "V",
	})
	bad := map[string]any{
		"label": "home", "recipient_name": "V", "recipient_phone": "123",
		"line1": "1 Road", "locality": "L", "city": "C", "state": "S", "postal_code": "560001",
	}
	if status, _ := env.Do(t, jar, "POST", "/api/v1/me/addresses", bad); status != http.StatusBadRequest {
		t.Fatalf("bad phone accepted (%d)", status)
	}
	bad["recipient_phone"] = "9876543210"
	bad["postal_code"] = "12"
	if status, _ := env.Do(t, jar, "POST", "/api/v1/me/addresses", bad); status != http.StatusBadRequest {
		t.Fatalf("bad pincode accepted (%d)", status)
	}
	// first address becomes default automatically
	status, res := env.Do(t, jar, "POST", "/api/v1/me/addresses", map[string]any{
		"label": "home", "recipient_name": "V", "recipient_phone": "9876543210",
		"line1": "1 Road", "locality": "Lakeview Enclave", "city": "Bitepur",
		"state": "Telengana Heights", "postal_code": "560002",
	})
	if status != http.StatusCreated {
		t.Fatalf("valid address rejected: %d", status)
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(res.Data, &created)

	var list []map[string]any
	status, res = env.Do(t, jar, "GET", "/api/v1/me/addresses", nil)
	_ = json.Unmarshal(res.Data, &list)
	if status != http.StatusOK || len(list) != 1 || list[0]["is_default"] != true {
		t.Fatalf("first address must become default (status %d list %v)", status, list)
	}
	_ = created
}

func TestAnonymousAccessDenied(t *testing.T) {
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/auth/session"},
		{"GET", "/api/v1/me"},
		{"GET", "/api/v1/me/addresses"},
		{"POST", "/api/v1/auth/logout"},
	} {
		status, _ := env.Do(t, env.NewJar(), tc.method, tc.path, nil)
		if status != http.StatusUnauthorized {
			t.Fatalf("%s %s anonymous = %d, want 401", tc.method, tc.path, status)
		}
	}
}

func TestRBACRoleEnforcement(t *testing.T) {
	// A synthetic admin-only route guarded by RequireRole proves the RBAC
	// middleware rejects customers and anonymous callers.
	authSvc := auth.NewService(env.Pool, "", false, env.Email)
	rg := httpx.NewRegistry()
	rg.Handle("GET", "/api/v1/admin-test/ping", httpx.RouteMeta{Summary: "admin ping", Tag: "test", Auth: "admin"},
		authSvc.RequireRole("admin")(func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteData(w, r, http.StatusOK, map[string]any{"pong": true})
		}))
	ts := newStandaloneServer(rg.Handler())
	defer ts.Close()

	// anonymous must be rejected
	if status := hitStandalone(ts, env.NewJar()); status != http.StatusUnauthorized {
		t.Fatalf("anonymous on admin route = %d, want 401", status)
	}

	// customer session must be rejected
	seedAdmin(t)
	customerJar := env.NewJar()
	_, _ = env.Do(t, customerJar, "POST", "/api/v1/auth/register", map[string]any{
		"email": "rbac-customer@example.test", "password": "Passw0rdLocal!", "name": "RBAC",
	})
	if status := hitStandalone(ts, customerJar); status != http.StatusForbidden {
		t.Fatalf("customer on admin route = %d, want 403", status)
	}

	// admin must pass
	adminJar := env.NewJar()
	if status, _ := env.Do(t, adminJar, "POST", "/api/v1/auth/login", map[string]any{
		"email": "admin@demo.biteorbit.local", "password": "Demo@1234",
	}); status != http.StatusOK {
		t.Fatalf("admin login failed (%d)", status)
	}
	if status := hitStandalone(ts, adminJar); status != http.StatusOK {
		t.Fatalf("admin on admin route = %d, want 200", status)
	}
}

// newStandaloneServer exposes an arbitrary handler for middleware-level tests.
func newStandaloneServer(h http.Handler) *httptest.Server {
	return httptest.NewServer(h)
}

// hitStandalone GETs /api/v1/admin-test/ping, carrying cookies from the main
// test environment (the standalone server runs on a different host:port, so
// the jar's host-scoped cookies are copied onto the request manually).
func hitStandalone(ts *httptest.Server, jar *cookiejar.Jar) int {
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/admin-test/ping", nil)
	if env != nil {
		envURL, _ := url.Parse(env.URL)
		for _, c := range jar.Cookies(envURL) {
			req.AddCookie(c)
		}
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return -1
	}
	res.Body.Close()
	return res.StatusCode
}

func seedAdmin(t *testing.T) {
	t.Helper()
	hash, err := auth.HashPassword("Demo@1234")
	if err != nil {
		t.Fatal(err)
	}
	_, err = env.Pool.Exec(t.Context(), `
		WITH u AS (
			INSERT INTO users (email, name) VALUES ('admin@demo.biteorbit.local','Test Admin') RETURNING id
		)
		INSERT INTO credentials (user_id, password_hash) SELECT id, $1 FROM u`, hash)
	if err != nil {
		// user may already exist from a previous test in this package run
		if !strings.Contains(err.Error(), "duplicate key") {
			t.Fatalf("seed admin: %v", err)
		}
	}
	_, err = env.Pool.Exec(t.Context(), `
		INSERT INTO user_roles (user_id, role)
		SELECT id, 'admin' FROM users WHERE email = 'admin@demo.biteorbit.local'
		ON CONFLICT DO NOTHING`)
	if err != nil {
		t.Fatalf("grant admin: %v", err)
	}
	// roles were granted AFTER the login session was cached — re-login required
}

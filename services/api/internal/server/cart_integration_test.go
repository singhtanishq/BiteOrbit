package server_test

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"testing"
)

// --- helpers ---------------------------------------------------------------

// findMenuItemByName walks the seeded catalogue and returns (itemID, outletID,
// modifierGroupsJSON) for the first menu item with that name.
func findMenuItemByName(t *testing.T, name string) (string, string, []map[string]any) {
	t.Helper()
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?limit=50", nil)
	if status != http.StatusOK {
		t.Fatalf("list outlets: %d", status)
	}
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	for _, o := range outlets {
		outletID, _ := o["id"].(string)
		status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/"+outletID+"/menu", nil)
		if status != http.StatusOK {
			continue
		}
		var menu struct {
			Categories []struct {
				Items []struct {
					ID             string           `json:"id"`
					Name           string           `json:"name"`
					ModifierGroups []map[string]any `json:"modifier_groups"`
				} `json:"items"`
			} `json:"categories"`
		}
		if err := json.Unmarshal(res.Data, &menu); err != nil {
			continue
		}
		for _, c := range menu.Categories {
			for _, it := range c.Items {
				if it.Name == name {
					return it.ID, outletID, it.ModifierGroups
				}
			}
		}
	}
	t.Fatalf("menu item %q not found in seed data", name)
	return "", "", nil
}

// loginAs signs in using the PROVIDED jar so its session cookie persists.
func loginAs(t *testing.T, jar *cookiejar.Jar, email string) {
	t.Helper()
	_, _ = env.Do(t, jar, "POST", "/api/v1/auth/login", map[string]any{
		"email": email, "password": "Passw0rdLocal!",
	})
}

// --- tests ------------------------------------------------------------------

func TestCartAddWithModifiers(t *testing.T) {
	email := "cart-mods@example.test"
	env.RegisterCustomer(t, email)

	biryaniID, _, groups := findMenuItemByName(t, "Dum Handi Chicken Biryani")

	// pick the second option of each multi-option group
	selections := map[string][]string{}
	expectedDelta := int64(0)
	for _, g := range groups {
		opts, _ := g["options"].([]any)
		if len(opts) > 1 {
			opt, _ := opts[1].(map[string]any)
			oid, _ := opt["id"].(string)
			gid, _ := g["id"].(string)
			delta, _ := opt["price_delta_paise"].(float64)
			selections[gid] = []string{oid}
			expectedDelta += int64(delta)
		}
	}

	jar := env.NewJar()
	loginAs(t, jar, email)

	status, res := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id":         biryaniID,
		"quantity":             2,
		"modifier_options":     selections,
		"special_instructions": "Extra raita, please pack separately.",
	})
	if status != http.StatusCreated {
		t.Fatalf("add to cart = %d (%v)", status, res.Error)
	}
	var cart struct {
		ItemsSubtotalPaise int64 `json:"items_subtotal_paise"`
		Items              []struct {
			Quantity            int   `json:"quantity"`
			UnitPricePaise      int64 `json:"unit_price_paise"`
			LineTotalPaise      int64 `json:"line_total_paise"`
			ModifiersTotalPaise int64 `json:"modifiers_total_paise"`
		} `json:"items"`
	}
	_ = json.Unmarshal(res.Data, &cart)
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 2 {
		t.Fatalf("cart should contain 1 line with qty 2: %+v", cart.Items)
	}
	it := cart.Items[0]
	if it.ModifiersTotalPaise != expectedDelta {
		t.Fatalf("modifiers total = %d, want %d", it.ModifiersTotalPaise, expectedDelta)
	}
	if it.LineTotalPaise != it.UnitPricePaise*2 || cart.ItemsSubtotalPaise != it.LineTotalPaise {
		t.Fatalf("totals mismatch: line=%d unit=%d subtotal=%d", it.LineTotalPaise, it.UnitPricePaise, cart.ItemsSubtotalPaise)
	}

	// fetch line id, update quantity, verify
	status, res = env.Do(t, jar, "GET", "/api/v1/cart", nil)
	var cartFull struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(res.Data, &cartFull)
	if len(cartFull.Items) == 0 {
		t.Fatal("cart lost its line")
	}
	lineID := cartFull.Items[0].ID

	status, res = env.Do(t, jar, "PATCH", "/api/v1/cart/items/"+lineID, map[string]any{"quantity": 3})
	if status != http.StatusOK {
		t.Fatalf("qty update = %d", status)
	}
	_ = json.Unmarshal(res.Data, &cart)
	if cart.Items[0].Quantity != 3 {
		t.Fatalf("qty not updated: %+v", cart.Items[0])
	}

	if status, _ = env.Do(t, jar, "PATCH", "/api/v1/cart/items/"+lineID, map[string]any{"quantity": 99}); status != http.StatusBadRequest {
		t.Fatalf("qty 99 accepted (%d)", status)
	}
	if status, _ = env.Do(t, jar, "DELETE", "/api/v1/cart/items/"+lineID, nil); status != http.StatusOK {
		t.Fatalf("delete line = %d", status)
	}
	if status, _ = env.Do(t, jar, "DELETE", "/api/v1/cart", nil); status != http.StatusOK {
		t.Fatalf("clear = %d", status)
	}
}

func TestCartCrossRestaurantConflict(t *testing.T) {
	email := "cart-conflict@example.test"
	env.RegisterCustomer(t, email)
	jar := env.NewJar()
	loginAs(t, jar, email)

	itemA, outletA, _ := findMenuItemByName(t, "Ghee Roast Dosa")
	itemB, _, _ := findMenuItemByName(t, "Pani Puri (6 pc)")

	if _, res := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemA, "quantity": 1,
	}); res.Error != nil {
		t.Fatalf("first add failed: %v", res.Error)
	}

	// adding from another restaurant must conflict with a clear explanation
	status, res := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemB, "quantity": 1,
	})
	if status != http.StatusConflict || res.Error == nil || res.Error.Code != "cart_conflict" {
		t.Fatalf("expected cart_conflict 409, got %d %v", status, res.Error)
	}
	if res.Error.Details["current_outlet"] == nil {
		t.Fatal("conflict must name the current outlet")
	}

	// explicit confirmation replaces the cart
	status, res = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": itemB, "quantity": 1, "replace_cart": true,
	})
	if status != http.StatusCreated {
		t.Fatalf("replace add = %d (%v)", status, res.Error)
	}
	var cart struct {
		OutletID string `json:"outlet_id"`
		Items    []any  `json:"items"`
	}
	_ = json.Unmarshal(res.Data, &cart)
	if cart.OutletID == outletA {
		t.Fatal("cart outlet not replaced")
	}
	if len(cart.Items) != 1 {
		t.Fatalf("old items must be gone, got %d lines", len(cart.Items))
	}
	_ = itemA
}

func TestCartValidationPaths(t *testing.T) {
	email := "cart-validate@example.test"
	env.RegisterCustomer(t, email)
	jar := env.NewJar()
	loginAs(t, jar, email)

	// unknown item
	status, _ := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": "00000000-0000-0000-0000-000000000000", "quantity": 1,
	})
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("unknown item = %d, want 422", status)
	}

	// quantity bounds
	dosaID, _, _ := findMenuItemByName(t, "Masala Dosa")
	status, _ = env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": dosaID, "quantity": 0,
	})
	if status != http.StatusBadRequest {
		t.Fatalf("qty 0 = %d, want 400", status)
	}

	// bogus modifier group/option
	status, res := env.Do(t, jar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": dosaID, "quantity": 1,
		"modifier_options": map[string][]string{
			"00000000-0000-0000-0000-000000000000": {"00000000-0000-0000-0000-000000000001"},
		},
	})
	if status != http.StatusBadRequest && status != http.StatusUnprocessableEntity {
		t.Fatalf("bogus modifiers accepted (%d) %v", status, res.Error)
	}

	// IDOR: a second user cannot touch the first user's cart line
	env.RegisterCustomer(t, "cart-victim@example.test")
	victimJar := env.NewJar()
	loginAs(t, victimJar, "cart-victim@example.test")
	_, _ = env.Do(t, victimJar, "POST", "/api/v1/cart/items", map[string]any{
		"menu_item_id": dosaID, "quantity": 1,
	})
	status, res = env.Do(t, victimJar, "GET", "/api/v1/cart", nil)
	var cart struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(res.Data, &cart)
	if len(cart.Items) == 0 {
		t.Fatal("victim cart empty")
	}
	lineID := cart.Items[0].ID

	env.RegisterCustomer(t, "cart-attacker@example.test")
	attackerJar := env.NewJar()
	loginAs(t, attackerJar, "cart-attacker@example.test")
	if status, _ = env.Do(t, attackerJar, "PATCH", "/api/v1/cart/items/"+lineID, map[string]any{"quantity": 1}); status != http.StatusNotFound {
		t.Fatalf("attacker patch on foreign cart line = %d, want 404", status)
	}
	if status, _ = env.Do(t, attackerJar, "DELETE", "/api/v1/cart/items/"+lineID, nil); status != http.StatusNotFound {
		t.Fatalf("attacker delete on foreign cart line = %d, want 404", status)
	}
}

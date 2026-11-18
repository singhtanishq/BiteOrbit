package server_test

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Discovery tests run against the seeded synthetic dataset.

func TestDiscoveryListRestaurants(t *testing.T) {
	env.SeedDemo(t)
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?page=1&limit=50", nil)
	if status != http.StatusOK {
		t.Fatalf("list status = %d", status)
	}
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	if len(outlets) < 15 {
		t.Fatalf("expected >=15 seeded outlets, got %d", len(outlets))
	}
	// meta pagination
	if res.Meta["total"].(float64) < 15 {
		t.Fatalf("meta.total missing: %v", res.Meta)
	}
	// first outlet has required card fields
	first := outlets[0]
	for _, key := range []string{"id", "name", "cuisines", "rating", "review_count", "is_open", "price_band"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("listing card missing %q: %v", key, first)
		}
	}
}

func TestDiscoveryFiltersAndSearch(t *testing.T) {
	// cuisine filter
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?cuisine=pizza", nil)
	if status != http.StatusOK {
		t.Fatalf("cuisine filter status = %d", status)
	}
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	if len(outlets) == 0 {
		t.Fatal("pizza filter returned no outlets")
	}
	for _, o := range outlets {
		found := false
		for _, c := range o["cuisines"].([]any) {
			if c.(string) == "Pizza" {
				found = true
			}
		}
		if !found {
			t.Fatalf("outlet %v lacks Pizza cuisine", o["name"])
		}
	}

	// text search over menu items ("biryani" must match Biryani Bandit)
	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?query=biryani", nil)
	if status != http.StatusOK {
		t.Fatalf("query search status = %d", status)
	}
	_ = json.Unmarshal(res.Data, &outlets)
	if len(outlets) == 0 {
		t.Fatal("biryani search returned no outlets")
	}

	// pure-veg filter
	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?veg=pure", nil)
	_ = json.Unmarshal(res.Data, &outlets)
	if status != http.StatusOK || len(outlets) == 0 {
		t.Fatalf("pure veg filter returned nothing (%d)", status)
	}
	for _, o := range outlets {
		if o["is_pure_veg"] != true {
			t.Fatalf("non-veg outlet in pure filter: %v", o["name"])
		}
	}
}

func TestDiscoveryPaginationBounds(t *testing.T) {
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?page=1&limit=999", nil)
	if status != http.StatusOK {
		t.Fatalf("status = %d", status)
	}
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	if res.Meta["limit"].(float64) > 50 {
		t.Fatalf("limit not bounded: %v", res.Meta["limit"])
	}
	_ = outlets
}

func TestDiscoverySuggestions(t *testing.T) {
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/suggestions?q=bir", nil)
	if status != http.StatusOK {
		t.Fatalf("suggestions status = %d", status)
	}
	var data struct {
		Restaurants []map[string]any `json:"restaurants"`
		Cuisines    []map[string]any `json:"cuisines"`
		Dishes      []map[string]any `json:"dishes"`
	}
	_ = json.Unmarshal(res.Data, &data)
	if len(data.Restaurants) == 0 && len(data.Cuisines) == 0 && len(data.Dishes) == 0 {
		t.Fatal("expected suggestions for 'bir'")
	}
	// short query returns empty sets without error
	status, _ = env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/suggestions?q=a", nil)
	if status != http.StatusOK {
		t.Fatalf("short query status = %d", status)
	}
}

func TestDiscoveryOutletDetailAndMenu(t *testing.T) {
	// pick the first outlet from the listing
	status, res := env.Do(t, env.NewJar(), "GET", "/api/v1/discovery/restaurants?page=1&limit=1", nil)
	var outlets []map[string]any
	_ = json.Unmarshal(res.Data, &outlets)
	id := outlets[0]["id"].(string)

	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/"+id, nil)
	if status != http.StatusOK {
		t.Fatalf("detail status = %d", status)
	}
	var detail map[string]any
	_ = json.Unmarshal(res.Data, &detail)
	for _, key := range []string{"name", "hours", "address", "rating", "is_open", "phone"} {
		if _, ok := detail[key]; !ok {
			t.Fatalf("detail missing %q", key)
		}
	}

	status, res = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/"+id+"/menu", nil)
	if status != http.StatusOK {
		t.Fatalf("menu status = %d", status)
	}
	var menu struct {
		Categories []struct {
			Name  string `json:"name"`
			Items []struct {
				Name        string `json:"name"`
				PricePaise  int64  `json:"price_paise"`
				IsVeg       bool   `json:"is_veg"`
				IsAvailable bool   `json:"is_available"`
				Modifiers   []struct {
					Name    string `json:"name"`
					Options []struct {
						Name            string `json:"name"`
						PriceDeltaPaise int64  `json:"price_delta_paise"`
					} `json:"options"`
				} `json:"modifier_groups"`
			} `json:"items"`
		} `json:"categories"`
	}
	if err := json.Unmarshal(res.Data, &menu); err != nil {
		t.Fatalf("menu decode: %v", err)
	}
	if len(menu.Categories) == 0 {
		t.Fatal("menu has no categories")
	}
	totalItems := 0
	for _, c := range menu.Categories {
		totalItems += len(c.Items)
	}
	if totalItems < 5 {
		t.Fatalf("expected a meaningful menu, got %d items", totalItems)
	}

	// unknown outlet 404s
	status, _ = env.Do(t, env.NewJar(), "GET", "/api/v1/restaurants/00000000-0000-0000-0000-000000000000/menu", nil)
	if status != http.StatusNotFound {
		t.Fatalf("unknown outlet = %d, want 404", status)
	}
}

func TestDiscoveryServiceability(t *testing.T) {
	// inside a seeded zone (Rosewood Hills center)
	status, res := env.Do(t, env.NewJar(), "POST", "/api/v1/discovery/serviceability", map[string]any{
		"lat": 17.4486, "lng": 78.4422,
	})
	if status != http.StatusOK {
		t.Fatalf("serviceability status = %d", status)
	}
	var data struct {
		Serviceable bool `json:"serviceable"`
		Zones       []struct {
			Name string `json:"name"`
		} `json:"zones"`
	}
	_ = json.Unmarshal(res.Data, &data)
	if !data.Serviceable {
		t.Fatal("Rosewood Hills center should be serviceable")
	}

	// far outside all zones
	status, res = env.Do(t, env.NewJar(), "POST", "/api/v1/discovery/serviceability", map[string]any{
		"lat": 19.0760, "lng": 72.8777, // Mumbai — outside demo city
	})
	_ = json.Unmarshal(res.Data, &data)
	if status != http.StatusOK || data.Serviceable {
		t.Fatalf("far point must be unserviceable (%d, %v)", status, data.Serviceable)
	}

	// neither point nor locality → validation error
	status, _ = env.Do(t, env.NewJar(), "POST", "/api/v1/discovery/serviceability", map[string]any{})
	if status != http.StatusBadRequest {
		t.Fatalf("empty body = %d, want 400", status)
	}
}

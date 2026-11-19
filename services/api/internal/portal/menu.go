package portal

import (
	"net/http"
	"time"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/httpx"
)

// OutletMenu serves GET /restaurant-portal/menu?outlet_id= — the same shape
// as the customer menu but without availability filtering, so staff see
// unavailable items too.
func (h *Handlers) OutletMenu(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	outletID := r.URL.Query().Get("outlet_id")
	if outletID == "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "outlet_id is required.", nil)
		return
	}
	if err := h.accessibleOutlet(r.Context(), h.pool, p.ID, outletID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type optDTO struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Delta   int64  `json:"price_delta_paise"`
		Default bool   `json:"is_default"`
		Avail   bool   `json:"is_available"`
	}
	type groupDTO struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Required bool     `json:"is_required"`
		Min      int      `json:"min_select"`
		Max      int      `json:"max_select"`
		Options  []optDTO `json:"options"`
	}
	type itemDTO struct {
		ID          string     `json:"id"`
		CategoryID  string     `json:"category_id"`
		Name        string     `json:"name"`
		Description string     `json:"description"`
		Price       int64      `json:"price_paise"`
		TaxCategory string     `json:"tax_category"`
		Veg         bool       `json:"is_veg"`
		Available   bool       `json:"is_available"`
		Featured    bool       `json:"is_featured"`
		Bestseller  bool       `json:"is_bestseller"`
		Allergens   *string    `json:"allergen_notes"`
		Groups      []groupDTO `json:"modifier_groups"`
	}
	type catDTO struct {
		ID    string    `json:"id"`
		Name  string    `json:"name"`
		Items []itemDTO `json:"items"`
	}

	cats := map[string]*catDTO{}
	order := []string{}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name FROM menu_categories WHERE outlet_id = $1 AND is_active
		ORDER BY display_order, name`, outletID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for rows.Next() {
		var c catDTO
		if rows.Scan(&c.ID, &c.Name) != nil {
			httpx.WriteError(w, r, err)
			return
		}
		cats[c.ID] = &c
		order = append(order, c.ID)
	}
	rows.Close()

	items := map[string][]itemDTO{}
	irows, err := h.pool.Query(r.Context(), `
		SELECT id, category_id, name, COALESCE(description,''), price_paise, tax_category, is_veg,
			is_available, is_featured, is_bestseller, allergen_notes
		FROM menu_items WHERE outlet_id = $1 AND deleted_at IS NULL
		ORDER BY display_order, name`, outletID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	itemIDs := []string{}
	for irows.Next() {
		var it itemDTO
		if err := irows.Scan(&it.ID, &it.CategoryID, &it.Name, &it.Description, &it.Price,
			&it.TaxCategory, &it.Veg, &it.Available, &it.Featured, &it.Bestseller, &it.Allergens); err != nil {
			irows.Close()
			httpx.WriteError(w, r, err)
			return
		}
		it.Groups = []groupDTO{}
		items[it.CategoryID] = append(items[it.CategoryID], it)
		itemIDs = append(itemIDs, it.ID)
	}
	irows.Close()

	if len(itemIDs) > 0 {
		grows, err := h.pool.Query(r.Context(), `
			SELECT img.item_id, mg.id, mg.name, mg.is_required, mg.min_select, mg.max_select
			FROM item_modifier_groups img JOIN modifier_groups mg ON mg.id = img.group_id
			WHERE img.item_id = ANY($1) ORDER BY img.display_order`, itemIDs)
		if err == nil {
			type gRow struct {
				itemID string
				group  groupDTO
			}
			var metas []gRow
			for grows.Next() {
				var m gRow
				if grows.Scan(&m.itemID, &m.group.ID, &m.group.Name, &m.group.Required,
					&m.group.Min, &m.group.Max) == nil {
					m.group.Options = []optDTO{}
					metas = append(metas, m)
				}
			}
			grows.Close()
			if len(metas) > 0 {
				gids := make([]string, len(metas))
				for i, m := range metas {
					gids[i] = m.group.ID
				}
				orows, err := h.pool.Query(r.Context(), `
					SELECT group_id, id, name, price_delta_paise, is_default, is_available
					FROM modifier_options WHERE group_id = ANY($1) ORDER BY id`, gids)
				if err == nil {
					opts := map[string][]optDTO{}
					for orows.Next() {
						var gid string
						var o optDTO
						if orows.Scan(&gid, &o.ID, &o.Name, &o.Delta, &o.Default, &o.Avail) == nil {
							opts[gid] = append(opts[gid], o)
						}
					}
					orows.Close()
					for i := range metas {
						metas[i].group.Options = opts[metas[i].group.ID]
					}
				}
			}
			for _, m := range metas {
				for ci := range items {
					for ii := range items[ci] {
						if items[ci][ii].ID == m.itemID {
							items[ci][ii].Groups = append(items[ci][ii].Groups, m.group)
						}
					}
				}
			}
		}
	}

	out := []catDTO{}
	for _, cid := range order {
		c := cats[cid]
		c.Items = items[cid]
		out = append(out, *c)
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"outlet_id": outletID, "categories": out})
}

type updateItemBody struct {
	Name         *string `json:"name"`
	Description  *string `json:"description"`
	PricePaise   *int64  `json:"price_paise"`
	IsAvailable  *bool   `json:"is_available"`
	IsFeatured   *bool   `json:"is_featured"`
	IsBestseller *bool   `json:"is_bestseller"`
}

// UpdateMenuItem serves PATCH /restaurant-portal/menu/items/{id}.
// Price and availability changes are audited through order-level audit logs.
func (h *Handlers) UpdateMenuItem(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")

	var body updateItemBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.PricePaise != nil && (*body.PricePaise < 100 || *body.PricePaise > 10000000) {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Price must be between ₹1 and ₹100,000.", nil)
		return
	}

	// authorise through the item's outlet
	var outletID string
	err := h.pool.QueryRow(r.Context(),
		`SELECT outlet_id FROM menu_items WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&outletID)
	if err != nil {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Menu item not found.", nil)
		return
	}
	if err = h.accessibleOutlet(r.Context(), h.pool, p.ID, outletID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// capture before-state for the audit trail
	var before struct {
		price int64
		avail bool
		name  string
	}
	_ = h.pool.QueryRow(r.Context(),
		`SELECT price_paise, is_available, name FROM menu_items WHERE id = $1`, id).
		Scan(&before.price, &before.avail, &before.name)

	sets := ""
	args := []any{}
	add := func(name string, v any) {
		args = append(args, v)
		if sets != "" {
			sets += ", "
		}
		sets += name + " = $" + itoa(len(args))
	}
	if body.Name != nil && *body.Name != "" {
		add("name", *body.Name)
	}
	if body.Description != nil {
		add("description", *body.Description)
	}
	if body.PricePaise != nil {
		add("price_paise", *body.PricePaise)
	}
	if body.IsAvailable != nil {
		add("is_available", *body.IsAvailable)
	}
	if body.IsFeatured != nil {
		add("is_featured", *body.IsFeatured)
	}
	if body.IsBestseller != nil {
		add("is_bestseller", *body.IsBestseller)
	}
	if sets == "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Nothing to update.", nil)
		return
	}
	add("updated_at", time.Now())
	args = append(args, id)
	tag, err := h.pool.Exec(r.Context(),
		`UPDATE menu_items SET `+sets+` WHERE id = $`+itoa(len(args)), args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Menu item not found.", nil)
		return
	}

	// audit trail for price/availability changes
	var afterPrice int64
	var afterAvail bool
	_ = h.pool.QueryRow(r.Context(),
		`SELECT price_paise, is_available FROM menu_items WHERE id = $1`, id).Scan(&afterPrice, &afterAvail)
	beforeJSON := `{"price_paise":` + itoa64(before.price) + `,"is_available":` + boolJSON(before.avail) + `}`
	afterJSON := `{"price_paise":` + itoa64(afterPrice) + `,"is_available":` + boolJSON(afterAvail) + `}`
	_, _ = h.pool.Exec(r.Context(), `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id, before_state, after_state)
		VALUES ('user', $1, 'menu_item.updated', 'menu_item', $2, $3::jsonb, $4::jsonb)`,
		p.ID, id, beforeJSON, afterJSON)

	httpx.WriteData(w, r, http.StatusOK, map[string]any{"updated": true})
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := ""
	if v < 0 {
		neg = "-"
		v = -v
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return neg + s
}

func itoa64(v int64) string { return itoa(int(v)) }

func boolJSON(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

var _ = apperr.CodeValidation

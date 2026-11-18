// Package discovery implements restaurant listing, search, suggestions,
// outlet detail and menu browsing. All data is read from the persisted
// catalogue; ratings come only from real (or demo-seeded, clearly labelled)
// review rows — never fabricated per-request.
package discovery

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/platform/geo"
)

// Handlers serves discovery endpoints.
type Handlers struct {
	pool *pgxpool.Pool
	geo  *geo.DemoAdapter
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool, geo: geo.NewDemo()}
}

// outletSummary is the listing card shape.
type outletSummary struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Description    string   `json:"description,omitempty"`
	Locality       string   `json:"locality"`
	City           string   `json:"city"`
	Cuisines       []string `json:"cuisines"`
	PriceBand      int      `json:"price_band"`
	MinOrderPaise  int64    `json:"min_order_paise"`
	IsPureVeg      bool     `json:"is_pure_veg"`
	PrepMinutesMin int      `json:"prep_minutes_min"`
	PrepMinutesMax int      `json:"prep_minutes_max"`
	Rating         float64  `json:"rating"`
	ReviewCount    int      `json:"review_count"`
	DistanceKm     *float64 `json:"distance_km,omitempty"`
	IsOpen         bool     `json:"is_open"`
	ClosedReason   string   `json:"closed_reason,omitempty"`
	ImageKey       *string  `json:"image_key,omitempty"`
}

// ListRestaurants serves GET /discovery/restaurants with filters, sorting and
// bounded pagination reflected in query parameters.
func (h *Handlers) ListRestaurants(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, limit := boundedPage(q.Get("page"), q.Get("limit"))
	offset := (page - 1) * limit

	conditions := []string{"o.deleted_at IS NULL", "o.status = 'active'"}
	args := []any{}
	arg := func(v any) string { args = append(args, v); return strconv.Itoa(len(args)) }

	if query := strings.TrimSpace(q.Get("query")); query != "" {
		p := arg("%" + query + "%")
		conditions = append(conditions, fmt.Sprintf(`(
			o.name ILIKE $%s OR o.description ILIKE $%s
			OR EXISTS (SELECT 1 FROM outlet_cuisines oc JOIN cuisine_types ct ON ct.id = oc.cuisine_id
				WHERE oc.outlet_id = o.id AND ct.name ILIKE $%s)
			OR EXISTS (SELECT 1 FROM menu_items mi WHERE mi.outlet_id = o.id AND mi.deleted_at IS NULL AND mi.name ILIKE $%s)
		)`, p, p, p, p))
	}
	if cuisine := strings.TrimSpace(q.Get("cuisine")); cuisine != "" {
		conditions = append(conditions, fmt.Sprintf(`EXISTS (
			SELECT 1 FROM outlet_cuisines oc JOIN cuisine_types ct ON ct.id = oc.cuisine_id
			WHERE oc.outlet_id = o.id AND ct.slug = $%s)`, arg(cuisine)))
	}
	if collection := strings.TrimSpace(q.Get("collection")); collection != "" {
		conditions = append(conditions, fmt.Sprintf(
			`EXISTS (SELECT 1 FROM outlet_collections oc2 WHERE oc2.outlet_id = o.id AND oc2.collection = $%s)`,
			arg(collection)))
	}
	switch strings.TrimSpace(q.Get("veg")) {
	case "true", "1":
		conditions = append(conditions, `(o.is_pure_veg OR EXISTS (
			SELECT 1 FROM menu_items mi WHERE mi.outlet_id = o.id AND mi.is_veg AND mi.deleted_at IS NULL))`)
	case "pure":
		conditions = append(conditions, `o.is_pure_veg`)
	}
	if pbMin := parseIntDefault(q.Get("price_min"), 0); pbMin > 0 {
		conditions = append(conditions, fmt.Sprintf(`o.price_band >= $%s`, arg(pbMin)))
	}
	if pbMax := parseIntDefault(q.Get("price_max"), 0); pbMax > 0 {
		conditions = append(conditions, fmt.Sprintf(`o.price_band <= $%s`, arg(pbMax)))
	}

	where := strings.Join(conditions, " AND ")
	order := `rating DESC NULLS LAST, o.name ASC`
	switch q.Get("sort") {
	case "rating":
		order = `rating DESC NULLS LAST, o.name ASC`
	case "delivery_time":
		order = `o.prep_minutes_min ASC, o.name ASC`
	case "price_asc":
		order = `o.price_band ASC, o.name ASC`
	case "price_desc":
		order = `o.price_band DESC, o.name ASC`
	case "name":
		order = `o.name ASC`
	}

	lat := parseFloat(q.Get("lat"))
	lng := parseFloat(q.Get("lng"))
	havePoint := lat != nil && lng != nil

	// NOTE: rating comes from real review rows only (demo seeds are labelled
	// synthetic in the dataset documentation).
	baseSelect := `
		SELECT o.id, o.name, COALESCE(o.description,''), o.locality, o.city, o.price_band,
			o.min_order_paise, o.is_pure_veg, o.prep_minutes_min, o.prep_minutes_max, o.image_key,
			COALESCE(ra.avg_rating, 0)::float8 AS rating, COALESCE(ra.review_count, 0)::int AS review_count,
			COALESCE((SELECT array_agg(ct.name ORDER BY ct.name) FROM outlet_cuisines oc
				JOIN cuisine_types ct ON ct.id = oc.cuisine_id WHERE oc.outlet_id = o.id), '{}'),
			o.lat::float8, o.lng::float8
		FROM outlets o
		LEFT JOIN (SELECT outlet_id, AVG(rating) avg_rating, COUNT(*) review_count
			FROM reviews WHERE status = 'published' GROUP BY outlet_id) ra ON ra.outlet_id = o.id
		WHERE ` + where + `
		ORDER BY ` + order + `
		LIMIT %d OFFSET %d`

	rows, err := h.pool.Query(r.Context(), fmt.Sprintf(baseSelect, limit, offset), args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()

	out := []outletSummary{}
	for rows.Next() {
		var s outletSummary
		var oLat, oLng float64
		if err := rows.Scan(&s.ID, &s.Name, &s.Description, &s.Locality, &s.City, &s.PriceBand,
			&s.MinOrderPaise, &s.IsPureVeg, &s.PrepMinutesMin, &s.PrepMinutesMax, &s.ImageKey,
			&s.Rating, &s.ReviewCount, &s.Cuisines, &oLat, &oLng); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if havePoint {
			// Distance from the requested point to the outlet (estimate only).
			d := h.geo.Estimate(geo.Point{Lat: *lat, Lng: *lng}, geo.Point{Lat: oLat, Lng: oLng})
			kd := d.DistanceKm
			s.DistanceKm = &kd
		}
		out = append(out, s)
	}
	if rows.Err() != nil {
		httpx.WriteError(w, r, rows.Err())
		return
	}

	// Open states are computed per outlet from persisted hours/holidays.
	h.openStates(r.Context(), out)

	var total int
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM outlets o WHERE `+where, args...).Scan(&total); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	httpx.WriteMeta(w, r, out, map[string]any{
		"page": page, "limit": limit, "total": total,
		"demo_note": "Distances and delivery estimates are simulated for the demo city of Bitepur.",
	})
}

// Suggestions serves GET /discovery/suggestions?q= for debounced search UIs.
func (h *Handlers) Suggestions(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(q) < 2 {
		httpx.WriteData(w, r, http.StatusOK, map[string]any{
			"restaurants": []any{}, "cuisines": []any{}, "dishes": []any{},
		})
		return
	}
	p := "%" + q + "%"
	prefix := q + "%"

	out := map[string]any{"restaurants": []any{}, "cuisines": []any{}, "dishes": []any{}}

	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, locality FROM outlets
		WHERE deleted_at IS NULL AND status = 'active' AND (name ILIKE $1 OR name ILIKE $2)
		ORDER BY CASE WHEN name ILIKE $2 THEN 0 ELSE 1 END, name LIMIT 6`, p, prefix)
	if err == nil {
		type s struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Locality string `json:"locality"`
		}
		list := []s{}
		for rows.Next() {
			var v s
			if rows.Scan(&v.ID, &v.Name, &v.Locality) == nil {
				list = append(list, v)
			}
		}
		rows.Close()
		out["restaurants"] = list
	}

	rows, err = h.pool.Query(r.Context(), `
		SELECT name, slug FROM cuisine_types WHERE name ILIKE $1 ORDER BY name LIMIT 4`, prefix)
	if err == nil {
		type c struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		list := []c{}
		for rows.Next() {
			var v c
			if rows.Scan(&v.Name, &v.Slug) == nil {
				list = append(list, v)
			}
		}
		rows.Close()
		out["cuisines"] = list
	}

	rows, err = h.pool.Query(r.Context(), `
		SELECT mi.id, mi.name, o.name AS outlet, mi.is_veg
		FROM menu_items mi JOIN outlets o ON o.id = mi.outlet_id
		WHERE mi.deleted_at IS NULL AND mi.is_available AND o.status = 'active'
			AND (mi.name ILIKE $1 OR mi.name ILIKE $2)
		ORDER BY CASE WHEN mi.name ILIKE $2 THEN 0 ELSE 1 END, mi.name LIMIT 6`, p, prefix)
	if err == nil {
		type d struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Outlet string `json:"outlet"`
			IsVeg  bool   `json:"is_veg"`
		}
		list := []d{}
		for rows.Next() {
			var v d
			if rows.Scan(&v.ID, &v.Name, &v.Outlet, &v.IsVeg) == nil {
				list = append(list, v)
			}
		}
		rows.Close()
		out["dishes"] = list
	}

	httpx.WriteData(w, r, http.StatusOK, out)
}

// SearchDishes serves GET /discovery/dishes?q= for the dish search tab.
func (h *Handlers) SearchDishes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	page, limit := boundedPage(q.Get("page"), q.Get("limit"))
	term := strings.TrimSpace(q.Get("query"))
	if term == "" {
		httpx.WriteMeta(w, r, []any{}, map[string]any{"page": page, "limit": limit, "total": 0})
		return
	}
	p := "%" + term + "%"

	var vegFilter string
	args := []any{p}
	switch q.Get("veg") {
	case "true", "1":
		vegFilter = " AND mi.is_veg"
	case "false", "0":
		vegFilter = " AND NOT mi.is_veg"
	}

	rows, err := h.pool.Query(r.Context(), `
		SELECT mi.id, mi.name, mi.description, mi.price_paise, mi.is_veg, o.id, o.name, o.locality
		FROM menu_items mi JOIN outlets o ON o.id = mi.outlet_id
		WHERE mi.deleted_at IS NULL AND mi.is_available AND o.status = 'active'
			AND (mi.name ILIKE $1 OR mi.description ILIKE $1)`+vegFilter+`
		ORDER BY mi.is_bestseller DESC, mi.name ASC
		LIMIT %d OFFSET %d`, append(args, limit, page-1)...)
	// LIMIT/OFFSET are integer-formatted constants, not user input.
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()

	type dish struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
		PricePaise  int64  `json:"price_paise"`
		IsVeg       bool   `json:"is_veg"`
		OutletID    string `json:"outlet_id"`
		OutletName  string `json:"outlet_name"`
		Locality    string `json:"locality"`
	}
	out := []dish{}
	for rows.Next() {
		var d dish
		if err := rows.Scan(&d.ID, &d.Name, &d.Description, &d.PricePaise, &d.IsVeg, &d.OutletID, &d.OutletName, &d.Locality); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		out = append(out, d)
	}

	var total int
	var vegExtra string
	if vegFilter != "" {
		vegExtra = vegFilter
	}
	if err := h.pool.QueryRow(r.Context(), `
		SELECT COUNT(*) FROM menu_items mi JOIN outlets o ON o.id = mi.outlet_id
		WHERE mi.deleted_at IS NULL AND mi.is_available AND o.status = 'active'
			AND (mi.name ILIKE $1 OR mi.description ILIKE $1)`+vegExtra, p).Scan(&total); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteMeta(w, r, out, map[string]any{"page": page, "limit": limit, "total": total})
}

// GetOutlet serves GET /restaurants/{id} — detail with hours and rating.
func (h *Handlers) GetOutlet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var s outletSummary
	var addr struct {
		Line1      string `json:"address_line1"`
		Locality   string `json:"locality"`
		City       string `json:"city"`
		State      string `json:"state"`
		PostalCode string `json:"postal_code"`
		Lat        float64
		Lng        float64
	}
	var phone string
	var orgName string
	err := h.pool.QueryRow(r.Context(), `
		SELECT o.id, o.name, COALESCE(o.description,''), o.locality, o.city, o.price_band,
			o.min_order_paise, o.is_pure_veg, o.prep_minutes_min, o.prep_minutes_max, o.image_key,
			COALESCE(ra.avg_rating, 0)::float8 AS rating, COALESCE(ra.review_count, 0)::int AS review_count,
			COALESCE((SELECT array_agg(ct.name ORDER BY ct.name) FROM outlet_cuisines oc
				JOIN cuisine_types ct ON ct.id = oc.cuisine_id WHERE oc.outlet_id = o.id), '{}'),
			o.phone, o.address_line1, o.state, o.postal_code, o.lat::float8, o.lng::float8,
			org.display_name
		FROM outlets o
		JOIN restaurant_organisations org ON org.id = o.organisation_id
		LEFT JOIN (SELECT outlet_id, AVG(rating) avg_rating, COUNT(*) review_count
			FROM reviews WHERE status = 'published' GROUP BY outlet_id) ra ON ra.outlet_id = o.id
		WHERE o.id = $1 AND o.deleted_at IS NULL`, id).Scan(
		&s.ID, &s.Name, &s.Description, &s.Locality, &s.City, &s.PriceBand,
		&s.MinOrderPaise, &s.IsPureVeg, &s.PrepMinutesMin, &s.PrepMinutesMax, &s.ImageKey,
		&s.Rating, &s.ReviewCount, &s.Cuisines,
		&phone, &addr.Line1, &addr.State, &addr.PostalCode, &addr.Lat, &addr.Lng, &orgName)
	if err == pgx.ErrNoRows {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Restaurant not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	open := h.openState(r.Context(), id)
	s.IsOpen = open.Open
	s.ClosedReason = open.Reason

	type hourRow struct {
		Weekday  int    `json:"weekday"`
		OpensAt  string `json:"opens_at"`
		ClosesAt string `json:"closes_at"`
		IsClosed bool   `json:"is_closed"`
	}
	hours := []hourRow{}
	hr, err := h.pool.Query(r.Context(), `
		SELECT weekday, opens_at::text, closes_at::text, is_closed
		FROM outlet_hours WHERE outlet_id = $1 ORDER BY weekday`, id)
	if err == nil {
		for hr.Next() {
			var row hourRow
			if hr.Scan(&row.Weekday, &row.OpensAt, &row.ClosesAt, &row.IsClosed) == nil {
				hours = append(hours, row)
			}
		}
		hr.Close()
	}

	// Collections for context chips.
	collections := []string{}
	cr, err := h.pool.Query(r.Context(),
		`SELECT collection FROM outlet_collections WHERE outlet_id = $1 ORDER BY collection`, id)
	if err == nil {
		for cr.Next() {
			var c string
			if cr.Scan(&c) == nil {
				collections = append(collections, c)
			}
		}
		cr.Close()
	}

	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"id": s.ID, "name": s.Name, "description": s.Description,
		"locality": s.Locality, "city": s.City, "cuisines": s.Cuisines,
		"price_band": s.PriceBand, "min_order_paise": s.MinOrderPaise,
		"is_pure_veg": s.IsPureVeg, "rating": s.Rating, "review_count": s.ReviewCount,
		"prep_minutes": map[string]int{"min": s.PrepMinutesMin, "max": s.PrepMinutesMax},
		"image_key":    s.ImageKey,
		"is_open":      s.IsOpen, "closed_reason": s.ClosedReason,
		"phone": phone, "organisation": orgName,
		"address": map[string]any{
			"line1": addr.Line1, "locality": s.Locality, "city": s.City,
			"state": addr.State, "postal_code": addr.PostalCode, "lat": addr.Lat, "lng": addr.Lng,
		},
		"hours":       hours,
		"collections": collections,
		"demo_note":   "Delivery estimates are simulated for the demo city; ratings come from seeded demo reviews (synthetic).",
	})
}

// GetOutletMenu serves GET /restaurants/{id}/menu — categories, items and
// modifier groups. Prices are authoritative from menu_items.
func (h *Handlers) GetOutletMenu(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	// outlet existence + basic state
	var name string
	err := h.pool.QueryRow(r.Context(),
		`SELECT name FROM outlets WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&name)
	if err == pgx.ErrNoRows {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Restaurant not found.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type modOption struct {
		ID              string `json:"id"`
		Name            string `json:"name"`
		PriceDeltaPaise int64  `json:"price_delta_paise"`
		IsDefault       bool   `json:"is_default"`
		IsAvailable     bool   `json:"is_available"`
	}
	type modGroup struct {
		ID         string      `json:"id"`
		Name       string      `json:"name"`
		IsRequired bool        `json:"is_required"`
		MinSelect  int         `json:"min_select"`
		MaxSelect  int         `json:"max_select"`
		Options    []modOption `json:"options"`
	}
	type item struct {
		ID             string     `json:"id"`
		CategoryID     string     `json:"category_id"`
		Name           string     `json:"name"`
		Description    string     `json:"description"`
		PricePaise     int64      `json:"price_paise"`
		TaxCategory    string     `json:"tax_category"`
		IsVeg          bool       `json:"is_veg"`
		DietaryTags    []string   `json:"dietary_tags"`
		AllergenNotes  *string    `json:"allergen_notes"`
		ServingInfo    *string    `json:"serving_info"`
		ImageKey       *string    `json:"image_key"`
		IsFeatured     bool       `json:"is_featured"`
		IsBestseller   bool       `json:"is_bestseller"`
		IsAvailable    bool       `json:"is_available"`
		ModifierGroups []modGroup `json:"modifier_groups"`
	}
	type category struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		DisplayOrder int    `json:"display_order"`
		Items        []item `json:"items"`
	}

	cats := map[string]*category{}
	order := []string{}
	rows, err := h.pool.Query(r.Context(), `
		SELECT id, name, display_order FROM menu_categories
		WHERE outlet_id = $1 AND is_active ORDER BY display_order, name`, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for rows.Next() {
		var c category
		if err := rows.Scan(&c.ID, &c.Name, &c.DisplayOrder); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		cats[c.ID] = &c
		order = append(order, c.ID)
	}
	rows.Close()

	items := map[string][]item{}
	irows, err := h.pool.Query(r.Context(), `
		SELECT id, category_id, name, COALESCE(description,''), price_paise, tax_category, is_veg,
			dietary_tags, allergen_notes, serving_info, image_key, is_featured, is_bestseller, is_available
		FROM menu_items WHERE outlet_id = $1 AND deleted_at IS NULL
		ORDER BY display_order, name`, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	itemOrder := []string{}
	for irows.Next() {
		var it item
		if err := irows.Scan(&it.ID, &it.CategoryID, &it.Name, &it.Description, &it.PricePaise,
			&it.TaxCategory, &it.IsVeg, &it.DietaryTags, &it.AllergenNotes, &it.ServingInfo,
			&it.ImageKey, &it.IsFeatured, &it.IsBestseller, &it.IsAvailable); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		it.ModifierGroups = []modGroup{}
		items[it.CategoryID] = append(items[it.CategoryID], it)
		itemOrder = append(itemOrder, it.ID)
	}
	irows.Close()

	// attach modifier groups in one query set
	mrows, err := h.pool.Query(r.Context(), `
		SELECT img.item_id, mg.id, mg.name, mg.is_required, mg.min_select, mg.max_select
		FROM item_modifier_groups img JOIN modifier_groups mg ON mg.id = img.group_id
		WHERE img.item_id = ANY($1) ORDER BY img.display_order`, itemOrder)
	if err == nil {
		type groupMeta struct {
			itemID string
			group  modGroup
		}
		var metas []groupMeta
		for mrows.Next() {
			var m groupMeta
			if mrows.Scan(&m.itemID, &m.group.ID, &m.group.Name, &m.group.IsRequired,
				&m.group.MinSelect, &m.group.MaxSelect) == nil {
				m.group.Options = []modOption{}
				metas = append(metas, m)
			}
		}
		mrows.Close()

		if len(metas) > 0 {
			groupIDs := make([]string, len(metas))
			for i, m := range metas {
				groupIDs[i] = m.group.ID
			}
			orows, err := h.pool.Query(r.Context(), `
				SELECT group_id, id, name, price_delta_paise, is_default, is_available
				FROM modifier_options WHERE group_id = ANY($1) ORDER BY id`, groupIDs)
			if err == nil {
				opts := map[string][]modOption{}
				for orows.Next() {
					var gid string
					var o modOption
					if orows.Scan(&gid, &o.ID, &o.Name, &o.PriceDeltaPaise, &o.IsDefault, &o.IsAvailable) == nil {
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
						items[ci][ii].ModifierGroups = append(items[ci][ii].ModifierGroups, m.group)
					}
				}
			}
		}
	}

	out := []category{}
	for _, cid := range order {
		c := cats[cid]
		c.Items = items[cid]
		if c.Items == nil {
			c.Items = []item{}
		}
		out = append(out, *c)
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"outlet_id": id, "outlet_name": name, "categories": out})
}

// ListCuisines serves GET /discovery/cuisines — filter chips source.
func (h *Handlers) ListCuisines(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT ct.name, ct.slug, COUNT(oc.outlet_id)
		FROM cuisine_types ct
		LEFT JOIN outlet_cuisines oc ON oc.cuisine_id = ct.id
		GROUP BY ct.id, ct.name, ct.slug
		HAVING COUNT(oc.outlet_id) > 0
		ORDER BY ct.name`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type c struct {
		Name    string `json:"name"`
		Slug    string `json:"slug"`
		Outlets int    `json:"outlets"`
	}
	out := []c{}
	for rows.Next() {
		var v c
		if rows.Scan(&v.Name, &v.Slug, &v.Outlets) == nil {
			out = append(out, v)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// ListCollections serves GET /discovery/collections — curated row source.
func (h *Handlers) ListCollections(w http.ResponseWriter, r *http.Request) {
	rows, err := h.pool.Query(r.Context(), `
		SELECT collection, COUNT(*) FROM outlet_collections GROUP BY collection ORDER BY collection`)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer rows.Close()
	type c struct {
		Name    string `json:"name"`
		Outlets int    `json:"outlets"`
	}
	out := []c{}
	for rows.Next() {
		var v c
		if rows.Scan(&v.Name, &v.Outlets) == nil {
			out = append(out, v)
		}
	}
	httpx.WriteData(w, r, http.StatusOK, out)
}

// Serviceability serves POST /discovery/serviceability — honest check of the
// demo zones for a point or locality name. Returns per-zone matches, never a
// fabricated "near you".
func (h *Handlers) Serviceability(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Lat      *float64 `json:"lat"`
		Lng      *float64 `json:"lng"`
		Locality string   `json:"locality"`
	}
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	type zone struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		City     string   `json:"city"`
		Distance *float64 `json:"distance_km,omitempty"`
		Serves   bool     `json:"serves"`
	}
	zones := []zone{}

	if body.Lat != nil && body.Lng != nil {
		rows, err := h.pool.Query(r.Context(), `
			SELECT id, name, city, center_lat::float8, center_lng::float8, radius_km::float8, is_active
			FROM service_zones WHERE is_active`)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var z zone
			var clat, clng, radius float64
			var active bool
			if err := rows.Scan(&z.ID, &z.Name, &z.City, &clat, &clng, &radius, &active); err != nil {
				httpx.WriteError(w, r, err)
				return
			}
			d := geo.DistanceKm(geo.Point{Lat: *body.Lat, Lng: *body.Lng}, geo.Point{Lat: clat, Lng: clng})
			dr := math.Round(d*10) / 10
			z.Distance = &dr
			z.Serves = d <= radius
			zones = append(zones, z)
		}
	} else if body.Locality != "" {
		rows, err := h.pool.Query(r.Context(), `
			SELECT id, name, city FROM service_zones WHERE is_active AND name ILIKE $1`,
			"%"+body.Locality+"%")
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		defer rows.Close()
		for rows.Next() {
			var z zone
			if err := rows.Scan(&z.ID, &z.Name, &z.City); err == nil {
				z.Serves = true
				zones = append(zones, z)
			}
		}
	} else {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Provide lat/lng or a locality name.", nil)
		return
	}

	serves := false
	for _, z := range zones {
		if z.Serves {
			serves = true
		}
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{
		"serviceable": serves,
		"zones":       zones,
		"demo_note":   "Zones are synthetic areas of the demo city Bitepur; distances are straight-line estimates.",
	})
}

// --- open-state helpers ---------------------------------------------------

type openState struct {
	Open   bool
	Reason string
}

const istLocation = "Asia/Kolkata"

func (h *Handlers) openState(ctx context.Context, outletID string) openState {
	outlets, err := h.loadOpenStates(ctx, []string{outletID})
	if err != nil || len(outlets) == 0 {
		return openState{Open: false, Reason: "unknown"}
	}
	return outlets[0].openState
}

func (h *Handlers) openStates(ctx context.Context, summaries []outletSummary) {
	ids := make([]string, len(summaries))
	for i, s := range summaries {
		ids[i] = s.ID
	}
	if len(ids) == 0 {
		return
	}
	states, err := h.loadOpenStates(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "open state computation failed", slog.Any("err", err))
		return
	}
	byID := map[string]openState{}
	for _, st := range states {
		byID[st.id] = st.openState
	}
	for i := range summaries {
		if st, ok := byID[summaries[i].ID]; ok {
			summaries[i].IsOpen = st.Open
			summaries[i].ClosedReason = st.Reason
		}
	}
}

type idOpen struct {
	id string
	openState
}

func (h *Handlers) loadOpenStates(ctx context.Context, ids []string) ([]idOpen, error) {
	loc, err := time.LoadLocation(istLocation)
	if err != nil {
		return nil, err
	}
	now := time.Now().In(loc) // weekday comes from the persisted hours rows

	type hoursRow struct {
		weekday  int
		outlet   string
		opensAt  string
		closesAt string
		closed   bool
	}
	hours := map[string][]hoursRow{}
	rows, err := h.pool.Query(ctx, `
		SELECT weekday, outlet_id, opens_at::text, closes_at::text, is_closed
		FROM outlet_hours WHERE outlet_id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var hr hoursRow
		if err := rows.Scan(&hr.weekday, &hr.outlet, &hr.opensAt, &hr.closesAt, &hr.closed); err == nil {
			hours[hr.outlet] = append(hours[hr.outlet], hr)
		}
	}
	rows.Close()

	holidays := map[string]bool{}
	hrows, err := h.pool.Query(ctx, `
		SELECT outlet_id FROM outlet_holidays
		WHERE outlet_id = ANY($1) AND holiday_date = (current_date AT TIME ZONE $2)`,
		ids, istLocation)
	if err == nil {
		for hrows.Next() {
			var id string
			if hrows.Scan(&id) == nil {
				holidays[id] = true
			}
		}
		hrows.Close()
	}

	// outlet status (active/inactive/suspended) also gates service
	status := map[string]string{}
	srows, err := h.pool.Query(ctx, `SELECT id, status FROM outlets WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	for srows.Next() {
		var id, st string
		if srows.Scan(&id, &st) == nil {
			status[id] = st
		}
	}
	srows.Close()

	out := []idOpen{}
	for _, id := range ids {
		if status[id] != "active" {
			out = append(out, idOpen{id, openState{Open: false, Reason: "temporarily unavailable"}})
			continue
		}
		if holidays[id] {
			out = append(out, idOpen{id, openState{Open: false, Reason: "closed today (holiday)"}})
			continue
		}
		today := int(now.Weekday()) // Sunday=0 matches the schema convention
		var todays []hoursRow
		for _, hr := range hours[id] {
			if hr.weekday == today {
				todays = append(todays, hr)
			}
		}
		if len(todays) == 0 {
			out = append(out, idOpen{id, openState{Open: false, Reason: "hours not published"}})
			continue
		}
		open := false
		reason := ""
		for _, hr := range todays {
			if hr.closed {
				continue
			}
			if withinWindow(now, hr.opensAt, hr.closesAt) {
				open = true
				break
			}
			reason = fmt.Sprintf("opens at %s", hr.opensAt)
		}
		if !open && reason == "" {
			reason = "closed for the day"
		}
		out = append(out, idOpen{id, openState{Open: open, Reason: reason}})
	}
	return out, nil
}

func withinWindow(now time.Time, opensAt, closesAt string) bool {
	o, err1 := parseClock(now, opensAt)
	c, err2 := parseClock(now, closesAt)
	if err1 != nil || err2 != nil {
		return false
	}
	if !c.After(o) {
		// overnight window (e.g., 10:00→02:00)
		return now.After(o) || now.Before(c)
	}
	return now.After(o) && now.Before(c)
}

func parseClock(now time.Time, hhmm string) (time.Time, error) {
	var hh, mm int
	if _, err := fmt.Sscanf(hhmm, "%d:%d", &hh, &mm); err != nil {
		return time.Time{}, err
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location()), nil
}

// --- small helpers --------------------------------------------------------

func boundedPage(pageStr, limitStr string) (int, int) {
	page := parseIntDefault(pageStr, 1)
	limit := parseIntDefault(limitStr, 20)
	if page < 1 {
		page = 1
	}
	if page > 200 {
		page = 200
	}
	if limit < 1 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	return page, limit
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func parseFloat(s string) *float64 {
	if s == "" {
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil
	}
	return &v
}

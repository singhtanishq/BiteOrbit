// Package cart implements the persistent single-restaurant cart with
// modifier validation. The server revalidates every price and availability
// against the live menu — client-supplied prices are never trusted.
package cart

import (
	"errors"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
)

type Handlers struct {
	pool *pgxpool.Pool
}

func NewHandlers(pool *pgxpool.Pool) *Handlers { return &Handlers{pool: pool} }

type cartItemDTO struct {
	ID                  string            `json:"id"`
	MenuItemID          string            `json:"menu_item_id"`
	Name                string            `json:"name"`
	IsVeg               bool              `json:"is_veg"`
	Quantity            int               `json:"quantity"`
	UnitPricePaise      int64             `json:"unit_price_paise"`
	LineTotalPaise      int64             `json:"line_total_paise"`
	SpecialInstructions *string           `json:"special_instructions"`
	Modifiers           []cartModifierDTO `json:"modifiers"`
	ModifiersTotalPaise int64             `json:"modifiers_total_paise"`
	AvailabilityState   string            `json:"availability_state"` // ok | unavailable | price_changed | missing
	CurrentPricePaise   int64             `json:"current_price_paise"`
	CurrentAvailable    bool              `json:"current_available"`
}

type cartModifierDTO struct {
	OptionID   string `json:"option_id"`
	GroupName  string `json:"group_name"`
	OptionName string `json:"option_name"`
	PriceDelta int64  `json:"price_delta_paise"`
}

type cartDTO struct {
	ID                 string        `json:"id"`
	OutletID           string        `json:"outlet_id"`
	OutletName         string        `json:"outlet_name"`
	OutletLocality     string        `json:"outlet_locality"`
	Items              []cartItemDTO `json:"items"`
	ItemsSubtotalPaise int64         `json:"items_subtotal_paise"`
	Note               *string       `json:"note"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

// Get returns the caller's active cart with server-side revalidation of
// availability and prices (snapshot vs current).
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	dto, err := h.loadCart(r, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if dto == nil {
		httpx.WriteData(w, r, http.StatusOK, nil) // no cart yet
		return
	}
	httpx.WriteData(w, r, http.StatusOK, dto)
}

type addItemBody struct {
	MenuItemID          string              `json:"menu_item_id"`
	Quantity            int                 `json:"quantity"`
	ModifierOptions     map[string][]string `json:"modifier_options"` // group_id -> option ids
	SpecialInstructions string              `json:"special_instructions"`
	ReplaceCart         bool                `json:"replace_cart"`
}

// AddItem adds a configured item. Cross-restaurant adds require explicit
// confirmation (replace_cart=true) — the existing cart is never silently
// erased.
func (h *Handlers) AddItem(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body addItemBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.Quantity < 1 || body.Quantity > 20 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Quantity must be between 1 and 20.", nil)
		return
	}
	if len(body.SpecialInstructions) > 240 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Special instructions are limited to 240 characters.", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	// live menu row (authoritative source for price/availability)
	var outletID string
	var name string
	var price int64
	var isVeg bool
	var available bool
	var deleted bool
	err = tx.QueryRow(r.Context(), `
		SELECT outlet_id, name, price_paise, is_veg, is_available, deleted_at IS NOT NULL
		FROM menu_items WHERE id = $1`, body.MenuItemID).Scan(
		&outletID, &name, &price, &isVeg, &available, &deleted)
	if errors.Is(err, pgx.ErrNoRows) || deleted {
		httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
			"This item is no longer on the menu.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if !available {
		httpx.Fail(w, r, http.StatusUnprocessableEntity, "unprocessable",
			"This item is currently unavailable.", nil)
		return
	}

	// validate modifiers against the item's groups
	modifierRows, err := validateModifiers(r, tx, body.MenuItemID, body.ModifierOptions)
	if err != nil {
		if appErr, ok := err.(*apperr.AppError); ok {
			httpx.Fail(w, r, appErr.Status, string(appErr.Code), appErr.Message, appErr.Details)
			return
		}
		httpx.WriteError(w, r, err)
		return
	}

	// existing cart (single active cart per user)
	var cartID, cartOutlet string
	err = tx.QueryRow(r.Context(),
		`SELECT id, outlet_id FROM carts WHERE user_id = $1 AND status = 'active'`, p.ID).Scan(&cartID, &cartOutlet)
	cartExists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, r, err)
		return
	}

	if cartExists && cartOutlet != outletID && !body.ReplaceCart {
		var otherOutlet string
		_ = tx.QueryRow(r.Context(), `SELECT name FROM outlets WHERE id = $1`, cartOutlet).Scan(&otherOutlet)
		httpx.Fail(w, r, http.StatusConflict, "cart_conflict",
			"Your cart has items from another restaurant.",
			map[string]any{"current_outlet": otherOutlet, "new_outlet_id": outletID})
		return
	}

	if !cartExists {
		err = tx.QueryRow(r.Context(),
			`INSERT INTO carts (user_id, outlet_id) VALUES ($1, $2) RETURNING id`, p.ID, outletID).Scan(&cartID)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	} else if cartOutlet != outletID {
		// confirmed replacement: drop old rows, retarget cart
		if _, err = tx.Exec(r.Context(),
			`DELETE FROM cart_items WHERE cart_id = $1`, cartID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		if _, err = tx.Exec(r.Context(),
			`UPDATE carts SET outlet_id = $2, updated_at = now() WHERE id = $1`, cartID, outletID); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}

	var itemID string
	modTotal := int64(0)
	for _, m := range modifierRows {
		modTotal += m.delta
	}
	err = tx.QueryRow(r.Context(), `
		INSERT INTO cart_items
		(cart_id, menu_item_id, quantity, special_instructions, name_snapshot, price_snapshot_paise, is_veg_snapshot)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6,$7) RETURNING id`,
		cartID, body.MenuItemID, body.Quantity, body.SpecialInstructions, name, price, isVeg).Scan(&itemID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	for _, m := range modifierRows {
		if _, err = tx.Exec(r.Context(), `
			INSERT INTO cart_item_modifiers
			(cart_item_id, group_id, option_id, group_name_snapshot, option_name_snapshot, price_delta_snapshot)
			VALUES ($1,$2,$3,$4,$5,$6)`,
			itemID, m.groupID, m.optionID, m.groupName, m.optionName, m.delta); err != nil {
			httpx.WriteError(w, r, err)
			return
		}
	}
	if _, err = tx.Exec(r.Context(),
		`UPDATE carts SET updated_at = now() WHERE id = $1`, cartID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	dto, err := h.loadCart(r, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusCreated, dto)
}

type validatedModifier struct {
	groupID    string
	optionID   string
	groupName  string
	optionName string
	delta      int64
}

// validateModifiers checks the client's selections against the item's
// modifier groups: required groups satisfied, min/max respected, options
// live and belonging to the right group. Defaults fill required groups.
func validateModifiers(r *http.Request, q db.Querier, itemID string, selections map[string][]string) ([]validatedModifier, error) {
	type groupRow struct {
		id       string
		name     string
		required bool
		min, max int
	}
	groups := map[string]groupRow{}
	var order []string
	rows, err := q.Query(r.Context(), `
		SELECT mg.id, mg.name, mg.is_required, mg.min_select, mg.max_select
		FROM item_modifier_groups img JOIN modifier_groups mg ON mg.id = img.group_id
		WHERE img.item_id = $1 ORDER BY img.display_order`, itemID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var g groupRow
		if err := rows.Scan(&g.id, &g.name, &g.required, &g.min, &g.max); err != nil {
			rows.Close()
			return nil, err
		}
		groups[g.id] = g
		order = append(order, g.id)
	}
	rows.Close()

	// all provided option ids must belong to groups of this item and be live
	type optRow struct {
		groupID   string
		name      string
		delta     int64
		available bool
	}
	opts := map[string]optRow{}
	if len(selections) > 0 {
		orows, err := q.Query(r.Context(), `
			SELECT mo.id, mo.group_id, mo.name, mo.price_delta_paise, mo.is_available
			FROM modifier_options mo
			JOIN item_modifier_groups img ON img.group_id = mo.group_id
			WHERE img.item_id = $1`, itemID)
		if err != nil {
			return nil, err
		}
		for orows.Next() {
			var o optRow
			var oid string
			if err := orows.Scan(&oid, &o.groupID, &o.name, &o.delta, &o.available); err != nil {
				orows.Close()
				return nil, err
			}
			opts[oid] = o
		}
		orows.Close()
	}

	chosen := map[string][]validatedModifier{}
	counts := map[string]int{}
	for groupID, optionIDs := range selections {
		g, ok := groups[groupID]
		if !ok {
			return nil, apperr.New(apperr.CodeValidation, "One of the selected option groups does not belong to this item.")
		}
		if len(optionIDs) > g.max && g.max > 0 {
			return nil, apperr.New(apperr.CodeValidation, g.name+": too many options selected.")
		}
		for _, oid := range optionIDs {
			o, ok := opts[oid]
			if !ok || o.groupID != groupID {
				return nil, apperr.New(apperr.CodeValidation, "An option does not belong to this item or group.")
			}
			if !o.available {
				return nil, apperr.New(apperr.CodeValidation, g.name+": an option is unavailable.")
			}
			chosen[groupID] = append(chosen[groupID], validatedModifier{
				groupID: groupID, optionID: oid, groupName: g.name, optionName: o.name, delta: o.delta,
			})
			counts[groupID]++
		}
	}
	// fill required groups from defaults when empty
	for _, gid := range order {
		g := groups[gid]
		if !g.required || counts[gid] >= g.min {
			continue
		}
		var defID string
		err := q.QueryRow(r.Context(), `
			SELECT id FROM modifier_options
			WHERE group_id = $1 AND is_available ORDER BY is_default DESC, id LIMIT 1`, gid).Scan(&defID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperr.New(apperr.CodeUnprocessable, g.name+" is required but has no available options.")
		}
		if err != nil {
			return nil, err
		}
		o := opts[defID]
		chosen[gid] = append(chosen[gid], validatedModifier{
			groupID: gid, optionID: defID, groupName: g.name, optionName: o.name, delta: o.delta,
		})
		counts[gid]++
	}
	// min_select check after default filling
	for _, gid := range order {
		g := groups[gid]
		if counts[gid] < g.min {
			return nil, apperr.New(apperr.CodeValidation, g.name+": select at least "+itoa(g.min)+".")
		}
	}

	var out []validatedModifier
	for _, gid := range order {
		out = append(out, chosen[gid]...)
	}
	return out, nil
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}

type updateItemBody struct {
	Quantity            *int    `json:"quantity"`
	SpecialInstructions *string `json:"special_instructions"`
}

// UpdateItem changes quantity/instructions for an owned cart line.
func (h *Handlers) UpdateItem(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	var body updateItemBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if body.Quantity != nil && (*body.Quantity < 1 || *body.Quantity > 20) {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Quantity must be between 1 and 20.", nil)
		return
	}
	if body.SpecialInstructions != nil && len(*body.SpecialInstructions) > 240 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Special instructions are limited to 240 characters.", nil)
		return
	}

	// ownership via join on carts.user_id
	var args []any
	sets := ""
	if body.Quantity != nil {
		args = append(args, *body.Quantity)
		sets = "quantity = $" + itoa(len(args))
	}
	if body.SpecialInstructions != nil {
		args = append(args, *body.SpecialInstructions)
		if sets != "" {
			sets += ", "
		}
		sets += "special_instructions = NULLIF($" + itoa(len(args)) + ",'')"
	}
	if sets == "" {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed", "Nothing to update.", nil)
		return
	}
	args = append(args, id, p.ID)
	tag, err := h.pool.Exec(r.Context(), `
		UPDATE cart_items ci SET `+sets+`
		FROM carts c
		WHERE c.id = ci.cart_id AND ci.id = $`+itoa(len(args)-1)+` AND c.user_id = $`+itoa(len(args)),
		args...)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Cart item not found.", nil)
		return
	}
	dto, err := h.loadCart(r, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, dto)
}

// DeleteItem removes one line from the caller's cart.
func (h *Handlers) DeleteItem(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	id := r.PathValue("id")
	tag, err := h.pool.Exec(r.Context(), `
		DELETE FROM cart_items ci
		USING carts c
		WHERE c.id = ci.cart_id AND ci.id = $2 AND c.user_id = $1`, p.ID, id)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Cart item not found.", nil)
		return
	}
	dto, err := h.loadCart(r, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, dto)
}

// Clear empties the caller's cart.
func (h *Handlers) Clear(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	_, err := h.pool.Exec(r.Context(), `
		DELETE FROM cart_items WHERE cart_id IN (SELECT id FROM carts WHERE user_id = $1 AND status='active')`, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, map[string]any{"cleared": true})
}

// loadCart builds the DTO with revalidation state for each line.
func (h *Handlers) loadCart(r *http.Request, userID string) (*cartDTO, error) {
	var dto cartDTO
	var cartID string
	err := h.pool.QueryRow(r.Context(), `
		SELECT c.id, c.outlet_id, o.name, o.locality, c.note, c.updated_at
		FROM carts c JOIN outlets o ON o.id = c.outlet_id
		WHERE c.user_id = $1 AND c.status = 'active'`, userID).Scan(
		&cartID, &dto.OutletID, &dto.OutletName, &dto.OutletLocality, &dto.Note, &dto.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dto.ID = cartID
	dto.Items = []cartItemDTO{}

	rows, err := h.pool.Query(r.Context(), `
		SELECT ci.id, ci.menu_item_id, ci.name_snapshot, ci.is_veg_snapshot, ci.quantity,
			ci.price_snapshot_paise, ci.special_instructions,
			mi.price_paise, mi.is_available, mi.deleted_at IS NOT NULL
		FROM cart_items ci JOIN menu_items mi ON mi.id = ci.menu_item_id
		WHERE ci.cart_id = $1 ORDER BY ci.created_at`, cartID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	subtotal := int64(0)
	for rows.Next() {
		var it cartItemDTO
		var snapPrice, livePrice int64
		var liveAvailable, deleted bool
		if err := rows.Scan(&it.ID, &it.MenuItemID, &it.Name, &it.IsVeg, &it.Quantity,
			&snapPrice, &it.SpecialInstructions, &livePrice, &liveAvailable, &deleted); err != nil {
			return nil, err
		}
		it.CurrentPricePaise = livePrice
		it.CurrentAvailable = liveAvailable && !deleted
		switch {
		case deleted || !liveAvailable:
			it.AvailabilityState = "unavailable"
			if deleted {
				it.AvailabilityState = "missing"
			}
		case snapPrice != livePrice:
			it.AvailabilityState = "price_changed"
		default:
			it.AvailabilityState = "ok"
		}

		// modifiers for this line
		mrows, err := h.pool.Query(r.Context(), `
			SELECT option_id, group_name_snapshot, option_name_snapshot, price_delta_snapshot
			FROM cart_item_modifiers WHERE cart_item_id = $1`, it.ID)
		if err != nil {
			return nil, err
		}
		it.Modifiers = []cartModifierDTO{}
		for mrows.Next() {
			var m cartModifierDTO
			if err := mrows.Scan(&m.OptionID, &m.GroupName, &m.OptionName, &m.PriceDelta); err != nil {
				mrows.Close()
				return nil, err
			}
			it.ModifiersTotalPaise += m.PriceDelta
			it.Modifiers = append(it.Modifiers, m)
		}
		mrows.Close()

		it.UnitPricePaise = snapPrice + it.ModifiersTotalPaise
		it.LineTotalPaise = it.UnitPricePaise * int64(it.Quantity)
		subtotal += it.LineTotalPaise
		dto.Items = append(dto.Items, it)
	}
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	dto.ItemsSubtotalPaise = subtotal
	return &dto, nil
}

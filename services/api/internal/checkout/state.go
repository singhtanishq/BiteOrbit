package checkout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
)

// cartState is the server-side view of the caller's active cart with LIVE
// authoritative prices from the menu (never cart snapshots).
type cartState struct {
	cartID      string
	outletID    string
	lines       []cartLine
	subtotal    int64
	fingerprint string
}

type cartLine struct {
	cartItemID string
	menuItemID string
	name       string
	isVeg      bool
	qty        int64
	unitPaise  int64 // live menu price + live modifier deltas
	tax        string
	instr      *string
}

type rawCartLine struct {
	cartItemID  string
	menuItemID  string
	name        string
	veg         bool
	qty         int64
	instr       *string
	price       int64
	tax         string
	unavailable bool
	deleted     bool
}

// loadCartState reads the cart and computes the fingerprint used for
// staleness detection between quote and placement.
func loadCartState(ctx context.Context, pool *pgxpool.Pool, userID string) (*cartState, error) {
	var st cartState
	err := pool.QueryRow(ctx,
		`SELECT id, outlet_id FROM carts WHERE user_id = $1 AND status = 'active'`, userID).
		Scan(&st.cartID, &st.outletID)
	if err != nil {
		return nil, translateNoRows(err, "Your cart is empty.")
	}

	raws := []rawCartLine{}
	rows, err := pool.Query(ctx, `
		SELECT ci.id, ci.menu_item_id, ci.name_snapshot, ci.is_veg_snapshot, ci.quantity,
			ci.special_instructions, mi.price_paise, mi.tax_category,
			NOT mi.is_available, mi.deleted_at IS NOT NULL
		FROM cart_items ci
		JOIN menu_items mi ON mi.id = ci.menu_item_id
		WHERE ci.cart_id = $1
		ORDER BY ci.created_at`, st.cartID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l rawCartLine
		if err := rows.Scan(&l.cartItemID, &l.menuItemID, &l.name, &l.veg, &l.qty,
			&l.instr, &l.price, &l.tax, &l.unavailable, &l.deleted); err != nil {
			rows.Close()
			return nil, err
		}
		raws = append(raws, l)
	}
	rows.Close()
	if rows.Err() != nil {
		return nil, rows.Err()
	}
	if len(raws) == 0 {
		return nil, apperr.New(apperr.CodeUnprocessable, "Your cart is empty.")
	}

	// live modifier deltas per line (options must still be available)
	type modRow struct {
		cartItemID string
		optionID   string
		delta      int64
	}
	mods := map[string][]modRow{}
	mrows, err := pool.Query(ctx, `
		SELECT cim.cart_item_id, cim.option_id, mo.price_delta_paise
		FROM cart_item_modifiers cim
		JOIN modifier_options mo ON mo.id = cim.option_id AND mo.is_available
		WHERE cim.cart_item_id = ANY($1)
		ORDER BY cim.option_id`, cartItemIDsOf(raws))
	if err != nil {
		return nil, err
	}
	for mrows.Next() {
		var m modRow
		if err := mrows.Scan(&m.cartItemID, &m.optionID, &m.delta); err != nil {
			mrows.Close()
			return nil, err
		}
		mods[m.cartItemID] = append(mods[m.cartItemID], m)
	}
	mrows.Close()
	if mrows.Err() != nil {
		return nil, mrows.Err()
	}

	fingerprintInput := ""
	for _, l := range raws {
		if l.deleted {
			return nil, apperr.New(apperr.CodeUnprocessable,
				"An item in your cart is no longer on the menu. Remove it to continue.")
		}
		if l.unavailable {
			return nil, apperr.New(apperr.CodeUnprocessable,
				"An item in your cart is currently unavailable. Remove it to continue.")
		}
		var optIDs []string
		deltas := int64(0)
		for _, m := range mods[l.cartItemID] {
			deltas += m.delta
			optIDs = append(optIDs, m.optionID)
		}
		st.lines = append(st.lines, cartLine{
			cartItemID: l.cartItemID,
			menuItemID: l.menuItemID,
			name:       l.name,
			isVeg:      l.veg,
			qty:        l.qty,
			unitPaise:  l.price + deltas,
			tax:        l.tax,
			instr:      l.instr,
		})
		st.subtotal += (l.price + deltas) * l.qty
		// canonical fingerprint input: item, qty, option ids (sorted by SQL), instructions
		fingerprintInput += fmt.Sprintf("%s|%d|%v|%s;", l.menuItemID, l.qty, optIDs, derefStr(l.instr))
	}
	sum := sha256.Sum256([]byte(fingerprintInput))
	st.fingerprint = hex.EncodeToString(sum[:])
	return &st, nil
}

func cartItemIDsOf(raws []rawCartLine) []string {
	out := make([]string, len(raws))
	for i, l := range raws {
		out[i] = l.cartItemID
	}
	return out
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

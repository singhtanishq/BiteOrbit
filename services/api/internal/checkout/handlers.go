// Package checkout implements quote creation and idempotent order placement.
// The server recomputes every authoritative amount at quote time, snapshots
// it, and re-verifies cart/quote/coupon state at placement.
package checkout

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/apperr"
	"biteorbit/services/api/internal/db"
	"biteorbit/services/api/internal/httpx"
	"biteorbit/services/api/internal/outbox"
	"biteorbit/services/api/internal/platform/geo"
	"biteorbit/services/api/internal/pricing"
)

const quoteTTL = 10 * time.Minute

// Handlers serves checkout endpoints.
type Handlers struct {
	pool *pgxpool.Pool
	geo  *geo.DemoAdapter
}

func NewHandlers(pool *pgxpool.Pool) *Handlers {
	return &Handlers{pool: pool, geo: geo.NewDemo()}
}

// translateNoRows converts pgx.ErrNoRows into an app error with the message.
func translateNoRows(err error, msg string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.New(apperr.CodeUnprocessable, msg)
	}
	return err
}

type quoteBody struct {
	AddressID     string `json:"address_id"`
	CouponCode    string `json:"coupon_code"`
	TipPaise      int64  `json:"tip_paise"`
	PaymentMethod string `json:"payment_method"` // cod | mock_upi | mock_card | mock_netbanking
}

type taxLineDTO struct {
	Category     string `json:"category"`
	RateBP       int64  `json:"rate_bp"`
	TaxablePaise int64  `json:"taxable_paise"`
	AmountPaise  int64  `json:"amount_paise"`
}

type breakdownDTO struct {
	ItemsSubtotalPaise  int64        `json:"items_subtotal_paise"`
	CouponDiscountPaise int64        `json:"coupon_discount_paise"`
	PackagingFeePaise   int64        `json:"packaging_fee_paise"`
	DeliveryFeePaise    int64        `json:"delivery_fee_paise"`
	PlatformFeePaise    int64        `json:"platform_fee_paise"`
	SmallOrderFeePaise  int64        `json:"small_order_fee_paise"`
	SurgeFeePaise       int64        `json:"surge_fee_paise"`
	TipPaise            int64        `json:"tip_paise"`
	TaxPaise            int64        `json:"tax_paise"`
	TaxLines            []taxLineDTO `json:"tax_lines"`
	GrandTotalPaise     int64        `json:"grand_total_paise"`
}

type couponDTO struct {
	Code          string `json:"code"`
	Title         string `json:"title"`
	DiscountPaise int64  `json:"discount_paise"`
	FundingParty  string `json:"funding_party"`
}

type quoteResponse struct {
	QuoteID          string       `json:"quote_id"`
	ExpiresAt        time.Time    `json:"expires_at"`
	OutletID         string       `json:"outlet_id"`
	OutletName       string       `json:"outlet_name"`
	AddressSummary   string       `json:"address_summary"`
	Breakdown        breakdownDTO `json:"breakdown"`
	Coupon           *couponDTO   `json:"coupon,omitempty"`
	DistanceKm       float64      `json:"distance_km"`
	ETAMinMinutes    int          `json:"eta_min_minutes"`
	ETAMaxMinutes    int          `json:"eta_max_minutes"`
	PaymentMethod    string       `json:"payment_method"`
	PaymentSimulated bool         `json:"payment_simulated"`
	DemoNote         string       `json:"demo_note"`
}

// Quote computes the full price sheet for the caller's CURRENT cart plus the
// chosen address, coupon and tip. Client input is limited to identifiers.
func (h *Handlers) Quote(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	var body quoteBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	switch body.PaymentMethod {
	case "cod", "mock_upi", "mock_card", "mock_netbanking":
	default:
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"payment_method must be cod, mock_upi, mock_card or mock_netbanking.", nil)
		return
	}
	if body.TipPaise < 0 || body.TipPaise > 100000 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"Tip must be between ₹0 and ₹1,000.", nil)
		return
	}

	resp, err := h.buildQuote(r.Context(), p.ID, body)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	httpx.WriteData(w, r, http.StatusOK, resp)
}

func (h *Handlers) buildQuote(ctx context.Context, userID string, body quoteBody) (*quoteResponse, error) {
	st, err := loadCartState(ctx, h.pool, userID)
	if err != nil {
		return nil, err
	}

	// outlet configuration
	var outletName string
	var packaging int64
	var minOrder int64
	var prepMin, prepMax int
	var oLat, oLng float64
	err = h.pool.QueryRow(ctx, `
		SELECT name, packaging_fee_paise, min_order_paise, prep_minutes_min, prep_minutes_max,
			lat::float8, lng::float8
		FROM outlets WHERE id = $1 AND status = 'active' AND deleted_at IS NULL`, st.outletID).Scan(
		&outletName, &packaging, &minOrder, &prepMin, &prepMax, &oLat, &oLng)
	if err = translateNoRows(err, "This restaurant is no longer taking orders."); err != nil {
		return nil, err
	}
	if st.subtotal < minOrder {
		return nil, apperr.New(apperr.CodeUnprocessable, fmt.Sprintf(
			"This restaurant has a minimum order of ₹%.2f.", float64(minOrder)/100))
	}

	// address (must belong to caller) + serviceability via demo zones
	var aLat, aLng *float64
	var label, line1, locality string
	var ignoreCity, ignoreState, ignorePostal string
	var ignoreName, ignorePhone string
	var ignoreInstructions *string
	err = h.pool.QueryRow(ctx, `
		SELECT lat::float8, lng::float8, label, line1, locality, city, state, postal_code,
			delivery_instructions, recipient_name, recipient_phone
		FROM customer_addresses WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`,
		body.AddressID, userID).Scan(&aLat, &aLng, &label, &line1, &locality, &ignoreCity,
		&ignoreState, &ignorePostal, &ignoreInstructions, &ignoreName, &ignorePhone)
	if err = translateNoRows(err, "Delivery address not found."); err != nil {
		return nil, err
	}
	_ = ignoreInstructions
	if aLat == nil || aLng == nil {
		// Resolve coordinates from the demo locality name → seeded zone centre.
		// (Locality names in the demo city map to service zones.)
		var zLat, zLng float64
		zerr := h.pool.QueryRow(ctx, `
			SELECT center_lat::float8, center_lng::float8 FROM service_zones
			WHERE is_active AND name ILIKE $1 LIMIT 1`, locality).Scan(&zLat, &zLng)
		if zerr != nil {
			return nil, apperr.New(apperr.CodeUnprocessable,
				"This address isn't inside a known demo locality. Pick a locality like Rosewood Hills.")
		}
		aLat = &zLat
		aLng = &zLng
	}
	zoneOK, distanceKm, err := h.serviceable(ctx, *aLat, *aLng, oLat, oLng)
	if err != nil {
		return nil, err
	}
	if !zoneOK {
		return nil, apperr.New(apperr.CodeUnprocessable,
			"This address is outside the restaurant's demo delivery area.")
	}
	est := h.geo.Estimate(geo.Point{Lat: *aLat, Lng: *aLng}, geo.Point{Lat: oLat, Lng: oLng})

	// pricing rule (active, effective now)
	var ruleID string
	var ruleVersion int
	var rawCfg []byte
	err = h.pool.QueryRow(ctx, `
		SELECT id, version, config FROM pricing_rules
		WHERE is_active AND effective_from <= now() AND (effective_to IS NULL OR effective_to > now())
		ORDER BY version DESC LIMIT 1`).Scan(&ruleID, &ruleVersion, &rawCfg)
	if err = translateNoRows(err, "Pricing is temporarily unavailable. Try again shortly."); err != nil {
		return nil, err
	}
	cfg, err := pricing.ParseConfig(rawCfg)
	if err != nil {
		return nil, apperr.New(apperr.CodeServiceUnavailable, "Pricing is temporarily unavailable. Try again shortly.")
	}

	// coupon (validated against the live subtotal)
	var coupon *pricing.Coupon
	if body.CouponCode != "" {
		coupon, err = pricing.ValidateCoupon(ctx, h.pool, body.CouponCode, st.outletID, st.subtotal, userID)
		var ce *pricing.CouponError
		if errors.As(err, &ce) {
			return nil, apperr.New(apperr.CodeUnprocessable, ce.Message).
				WithDetails(map[string]any{"coupon_code": ce.Code})
		}
		if err != nil {
			return nil, err
		}
		if err != nil {
			return nil, err
		}
	}

	lines := make([]pricing.LineInput, len(st.lines))
	for i, l := range st.lines {
		lines[i] = pricing.LineInput{UnitPaise: l.unitPaise, Qty: l.qty, TaxCategory: l.tax}
	}
	breakdown := pricing.Quote(cfg, pricing.QuoteInput{
		Lines:          lines,
		PackagingPaise: packaging,
		DistanceKm:     distanceKm,
		TipPaise:       body.TipPaise,
		CouponDiscount: couponDiscountOf(coupon),
	})

	// persist the quote with its fingerprint, method and ETA
	var quoteID string
	var expiresAt time.Time
	bdRaw, _ := json.Marshal(breakdown)
	err = h.pool.QueryRow(ctx, `
		INSERT INTO pricing_quotes
		(user_id, outlet_id, cart_fingerprint, address_id, coupon_id, pricing_rule_id, breakdown,
		 grand_total_paise, version, expires_at, payment_method, eta_min_minutes, eta_max_minutes)
		VALUES ($1,$2,$3,$4,$5,$6,$7::jsonb,$8,$9, now() + interval '10 minutes', $10,$11,$12)
		RETURNING id, expires_at`,
		userID, st.outletID, st.fingerprint, body.AddressID, couponIDOf(coupon), ruleID,
		string(bdRaw), breakdown.GrandTotalPaise, ruleVersion, body.PaymentMethod,
		prepMin+est.MinutesMin, prepMax+est.MinutesMax).Scan(&quoteID, &expiresAt)
	if err != nil {
		return nil, err
	}

	return &quoteResponse{
		QuoteID:          quoteID,
		ExpiresAt:        expiresAt,
		OutletID:         st.outletID,
		OutletName:       outletName,
		AddressSummary:   label + " · " + line1 + ", " + locality,
		Breakdown:        toBreakdownDTO(breakdown),
		Coupon:           toCouponDTO(coupon),
		DistanceKm:       distanceKm,
		ETAMinMinutes:    prepMin + est.MinutesMin,
		ETAMaxMinutes:    prepMax + est.MinutesMax,
		PaymentMethod:    body.PaymentMethod,
		PaymentSimulated: body.PaymentMethod != "cod",
		DemoNote:         "Payment is a clearly labelled local simulation. No real money moves.",
	}, nil
}

// serviceable checks the address against active demo zones; distance is the
// straight-line outlet→address estimate (labelled as such by callers).
func (h *Handlers) serviceable(ctx context.Context, aLat, aLng, oLat, oLng float64) (bool, float64, error) {
	rows, err := h.pool.Query(ctx, `
		SELECT center_lat::float8, center_lng::float8, radius_km::float8
		FROM service_zones WHERE is_active`)
	if err != nil {
		return false, 0, err
	}
	defer rows.Close()
	ok := false
	for rows.Next() {
		var zlat, zlng, radius float64
		if err := rows.Scan(&zlat, &zlng, &radius); err != nil {
			return false, 0, err
		}
		if geo.DistanceKm(geo.Point{Lat: aLat, Lng: aLng}, geo.Point{Lat: zlat, Lng: zlng}) <= radius {
			ok = true
		}
	}
	if rows.Err() != nil {
		return false, 0, rows.Err()
	}
	d := geo.DistanceKm(geo.Point{Lat: aLat, Lng: aLng}, geo.Point{Lat: oLat, Lng: oLng})
	return ok, d, nil
}

// --- placement ------------------------------------------------------------

type placeBody struct {
	QuoteID string `json:"quote_id"`
}

type placeResponse struct {
	OrderID       string    `json:"order_id"`
	Reference     string    `json:"reference"`
	Status        string    `json:"status"`
	GrandTotal    int64     `json:"grand_total_paise"`
	PaymentMethod string    `json:"payment_method"`
	PaymentStatus string    `json:"payment_status"`
	PaymentID     string    `json:"payment_id"`
	ETAMinMinutes int       `json:"eta_min_minutes"`
	ETAMaxMinutes int       `json:"eta_max_minutes"`
	PlacedAt      time.Time `json:"placed_at"`
}

// Place converts a valid quote into an order. Requires an Idempotency-Key
// header: retried submissions with the same key replay the stored response
// instead of creating a second order.
func (h *Handlers) Place(w http.ResponseWriter, r *http.Request) {
	p := httpx.UserFrom(r.Context())
	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" || len(idempotencyKey) > 120 {
		httpx.Fail(w, r, http.StatusBadRequest, "validation_failed",
			"An Idempotency-Key header is required for order placement.", nil)
		return
	}

	var body placeBody
	if err := httpx.DecodeJSON(w, r, &body); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// request hash binds the key to this exact request
	bodyRaw, _ := json.Marshal(body)
	hm := sha256.Sum256(append([]byte(p.ID+"|place|"), bodyRaw...))
	requestHash := hex.EncodeToString(hm[:])

	// replay path (same key + same request)
	var storedStatus int
	var storedBody []byte
	var storedHash string
	err := h.pool.QueryRow(r.Context(), `
		SELECT COALESCE(response_status, 0), COALESCE(response_body::text, ''), request_hash
		FROM idempotency_records WHERE key = $1 AND expires_at > now()`, idempotencyKey).
		Scan(&storedStatus, &storedBody, &storedHash)
	if err == nil {
		if subtle.ConstantTimeCompare([]byte(storedHash), []byte(requestHash)) == 1 {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(storedStatus)
			_, _ = w.Write(storedBody)
			return
		}
		httpx.Fail(w, r, http.StatusConflict, "idempotency_replayed",
			"This idempotency key was already used with a different request.", nil)
		return
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		httpx.WriteError(w, r, err)
		return
	}

	// quote (must belong to caller, not expired)
	var q struct {
		id          string
		outletID    string
		fingerprint string
		addressID   *string
		couponID    *string
		ruleID      *string
		breakdown   []byte
		grandTotal  int64
		version     int
		expiresAt   time.Time
		method      string
		etaMin      int
		etaMax      int
	}
	err = h.pool.QueryRow(r.Context(), `
		SELECT id, outlet_id, cart_fingerprint, address_id, coupon_id, pricing_rule_id,
			breakdown, grand_total_paise, version, expires_at, payment_method,
			COALESCE(eta_min_minutes, 30), COALESCE(eta_max_minutes, 55)
		FROM pricing_quotes WHERE id = $1 AND user_id = $2`, body.QuoteID, p.ID).Scan(
		&q.id, &q.outletID, &q.fingerprint, &q.addressID, &q.couponID, &q.ruleID,
		&q.breakdown, &q.grandTotal, &q.version, &q.expiresAt, &q.method, &q.etaMin, &q.etaMax)
	if errors.Is(err, pgx.ErrNoRows) {
		// user-scoped lookup: a foreign quote is indistinguishable from a missing one
		httpx.Fail(w, r, http.StatusNotFound, "not_found", "Quote not found — request a new quote.", nil)
		return
	}
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if time.Now().After(q.expiresAt) {
		httpx.Fail(w, r, http.StatusConflict, "conflict",
			"This quote expired. Review your cart and request a fresh quote.", nil)
		return
	}

	// cart unchanged since quote? (staleness/tamper detection)
	st, err := loadCartState(r.Context(), h.pool, p.ID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(st.fingerprint), []byte(q.fingerprint)) != 1 ||
		st.outletID != q.outletID {
		httpx.Fail(w, r, http.StatusConflict, "conflict",
			"Your cart changed after this quote was created. Please review and re-quote.", nil)
		return
	}

	tx, err := h.pool.Begin(r.Context())
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	defer tx.Rollback(r.Context())

	reference, err := newReference(r.Context(), tx)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// authoritative price snapshot on the order
	bd := map[string]any{}
	_ = json.Unmarshal(q.breakdown, &bd)
	pricingSnapshot := map[string]any{
		"rule_id":      q.ruleID,
		"rule_version": q.version,
		"breakdown":    bd,
		"quote_id":     q.id,
		"fingerprint":  q.fingerprint,
		"captured_at":  time.Now().UTC(),
	}
	snapshotRaw, _ := json.Marshal(pricingSnapshot)

	status := "placed"
	if q.method != "cod" {
		status = "pending_payment"
	}

	var orderID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO orders
		(reference, customer_id, outlet_id, organisation_id, status, mode, placed_at,
		 items_subtotal_paise, coupon_discount_paise, packaging_fee_paise, delivery_fee_paise,
		 platform_fee_paise, small_order_fee_paise, surge_fee_paise, tip_paise, tax_paise,
		 grand_total_paise, pricing_snapshot, pricing_rule_id, quote_id, coupon_id,
		 eta_min_minutes, eta_max_minutes, instructions, created_at, updated_at)
		VALUES ($1,$2,$3,(SELECT organisation_id FROM outlets WHERE id=$3),$4,'delivery',now(),
		 $5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::jsonb,$16,$17,$18,$19,$20,
		 (SELECT note FROM carts WHERE user_id = $2 AND status='active'), now(), now())
		RETURNING id`,
		reference, p.ID, q.outletID, status,
		intOf(bd, "items_subtotal_paise"), intOf(bd, "coupon_discount_paise"),
		intOf(bd, "packaging_fee_paise"), intOf(bd, "delivery_fee_paise"),
		intOf(bd, "platform_fee_paise"), intOf(bd, "small_order_fee_paise"),
		intOf(bd, "surge_fee_paise"), intOf(bd, "tip_paise"), intOf(bd, "tax_paise"),
		q.grandTotal, string(snapshotRaw), nullStr(q.ruleID), q.id, nullStr(q.couponID),
		q.etaMin, q.etaMax).Scan(&orderID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// order items (+modifier snapshots) and address snapshot
	if err = h.copyCartItems(r.Context(), tx, st.cartID, orderID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = h.copyOrderAddress(r.Context(), tx, q.addressID, orderID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// payment intent: online → pending (captured later via mock provider); COD → cod_due
	paymentStatus := "pending"
	if q.method == "cod" {
		paymentStatus = "cod_due"
	}
	var paymentID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO payments (order_id, provider, method, status, amount_paise, currency, provider_reference)
		VALUES ($1,'mock',$2,$3,$4,'INR',$5) RETURNING id`,
		orderID, q.method, paymentStatus, q.grandTotal, "mockpay_"+reference).Scan(&paymentID)
	if err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO payment_attempts (payment_id, attempt_no, status, provider_reference, detail)
		VALUES ($1,1,$2,$3,'{"simulated":true}'::jsonb)`, paymentID, paymentStatus, "mockpay_"+reference); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// coupon redemption (race-safe, inside the placement transaction)
	if q.couponID != nil {
		if err = pricing.RedeemCoupon(r.Context(), tx, *q.couponID, p.ID, orderID,
			intOf(bd, "coupon_discount_paise")); err != nil {
			var ce *pricing.CouponError
			if errors.As(err, &ce) {
				httpx.Fail(w, r, http.StatusConflict, "conflict", ce.Message, nil)
				return
			}
			httpx.WriteError(w, r, err)
			return
		}
	}

	// event trail + outbox
	if _, err = tx.Exec(r.Context(), `
		INSERT INTO order_events (order_id, event_type, from_state, to_state, actor_type, actor_id)
		VALUES ($1,'state_changed',NULL,$2,'customer',$3)`, orderID, status, p.ID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}
	if err = outbox.Enqueue(r.Context(), tx, "order", orderID, "order.status_changed", map[string]any{
		"order_id": orderID, "reference": reference, "customer_id": p.ID,
		"from_state": "", "to_state": status,
	}, ptr(fmt.Sprintf("order-status:%s:%s", orderID, status))); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	// convert the cart
	if _, err = tx.Exec(r.Context(),
		`UPDATE carts SET status = 'converted', updated_at = now() WHERE id = $1`, st.cartID); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	if err = tx.Commit(r.Context()); err != nil {
		httpx.WriteError(w, r, err)
		return
	}

	resp := placeResponse{
		OrderID:       orderID,
		Reference:     reference,
		Status:        status,
		GrandTotal:    q.grandTotal,
		PaymentMethod: q.method,
		PaymentStatus: paymentStatus,
		PaymentID:     paymentID,
		ETAMinMinutes: q.etaMin,
		ETAMaxMinutes: q.etaMax,
		PlacedAt:      time.Now().UTC(),
	}
	respRaw, _ := json.Marshal(map[string]any{"data": resp, "request_id": httpx.RequestIDFrom(r.Context())})

	// store the idempotency record (unique key guarantees no duplicate orders)
	_, _ = h.pool.Exec(r.Context(), `
		INSERT INTO idempotency_records (key, endpoint, user_id, request_hash, response_status, response_body, expires_at)
		VALUES ($1,'POST /api/v1/checkout/place',$2,$3,$4,$5::jsonb, now() + interval '24 hours')
		ON CONFLICT (key) DO NOTHING`,
		idempotencyKey, p.ID, requestHash, http.StatusOK, string(respRaw))

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(respRaw)
}

// copyCartItems snapshots cart lines into order_items.
func (h *Handlers) copyCartItems(ctx context.Context, tx db.Querier, cartID, orderID string) error {
	rows, err := tx.Query(ctx, `
		SELECT ci.id, ci.menu_item_id, ci.name_snapshot, ci.is_veg_snapshot, ci.quantity,
			ci.special_instructions,
			mi.price_paise + COALESCE((
				SELECT SUM(mo.price_delta_paise) FROM cart_item_modifiers cim
				JOIN modifier_options mo ON mo.id = cim.option_id
				WHERE cim.cart_item_id = ci.id AND mo.is_available
			), 0),
			COALESCE((
				SELECT SUM(mo.price_delta_paise) FROM cart_item_modifiers cim
				JOIN modifier_options mo ON mo.id = cim.option_id
				WHERE cim.cart_item_id = ci.id AND mo.is_available
			), 0)
		FROM cart_items ci JOIN menu_items mi ON mi.id = ci.menu_item_id
		WHERE ci.cart_id = $1 ORDER BY ci.created_at`, cartID)
	if err != nil {
		return err
	}
	type line struct {
		cartItemID      string
		itemID          string
		name            string
		veg             bool
		qty, unit, mods int64
		instr           *string
	}
	var lines []line
	for rows.Next() {
		var l line
		if err := rows.Scan(&l.cartItemID, &l.itemID, &l.name, &l.veg, &l.qty, &l.instr,
			&l.unit, &l.mods); err != nil {
			rows.Close()
			return err
		}
		lines = append(lines, l)
	}
	rows.Close()
	if rows.Err() != nil {
		return rows.Err()
	}

	for _, l := range lines {
		var orderItemID string
		err := tx.QueryRow(ctx, `
			INSERT INTO order_items
			(order_id, menu_item_id, name_snapshot, is_veg_snapshot, quantity, unit_price_paise,
			 item_total_paise, special_instructions, modifiers_total_paise)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`,
			orderID, l.itemID, l.name, l.veg, l.qty, l.unit, l.unit*l.qty, l.instr, l.mods).Scan(&orderItemID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO order_item_modifiers (order_item_id, group_name_snapshot, option_name_snapshot, price_delta_paise)
			SELECT $1, cim.group_name_snapshot, cim.option_name_snapshot, cim.price_delta_snapshot
			FROM cart_item_modifiers cim WHERE cim.cart_item_id = $2`, orderItemID, l.cartItemID); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handlers) copyOrderAddress(ctx context.Context, tx db.Querier, addressID *string, orderID string) error {
	if addressID == nil {
		return apperr.New(apperr.CodeUnprocessable, "A delivery address is required.")
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO order_addresses
		(order_id, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, instructions, zone_id)
		SELECT $2, ca.recipient_name, ca.recipient_phone, ca.line1, ca.line2, ca.landmark, ca.locality, ca.city, ca.state, ca.postal_code,
			ca.lat, ca.lng, ca.delivery_instructions,
			(SELECT sz.id FROM service_zones sz
			 WHERE sz.is_active AND ca.lat IS NOT NULL AND ca.lng IS NOT NULL
			   AND geo_distance_km(ca.lat::float8, ca.lng::float8, sz.center_lat::float8, sz.center_lng::float8) <= sz.radius_km
			 LIMIT 1)
		FROM customer_addresses ca WHERE ca.id = $1 AND ca.deleted_at IS NULL`,
		*addressID, orderID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return apperr.New(apperr.CodeUnprocessable, "Delivery address not found.")
	}
	return nil
}

func newReference(ctx context.Context, tx db.Querier) (string, error) {
	const alphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"
	for attempt := 0; attempt < 5; attempt++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))*36*36*36*36*36))
		if err != nil {
			return "", err
		}
		ref := "BO-"
		v := n.Int64()
		for i := 0; i < 6; i++ {
			ref += string(alphabet[v%int64(len(alphabet))])
			v /= int64(len(alphabet))
		}
		var exists bool
		if err := tx.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM orders WHERE reference = $1)`, ref).Scan(&exists); err != nil {
			return "", err
		}
		if !exists {
			return ref, nil
		}
	}
	return "", fmt.Errorf("could not allocate order reference")
}

// --- small helpers --------------------------------------------------------

func couponDiscountOf(c *pricing.Coupon) int64 {
	if c == nil {
		return 0
	}
	return c.DiscountPaise
}

func couponIDOf(c *pricing.Coupon) *string {
	if c == nil {
		return nil
	}
	return &c.ID
}

func toCouponDTO(c *pricing.Coupon) *couponDTO {
	if c == nil {
		return nil
	}
	return &couponDTO{Code: c.Code, Title: c.Title, DiscountPaise: c.DiscountPaise, FundingParty: c.FundingParty}
}

func toBreakdownDTO(b pricing.Breakdown) breakdownDTO {
	dto := breakdownDTO{
		ItemsSubtotalPaise:  b.ItemsSubtotalPaise,
		CouponDiscountPaise: b.CouponDiscountPaise,
		PackagingFeePaise:   b.PackagingFeePaise,
		DeliveryFeePaise:    b.DeliveryFeePaise,
		PlatformFeePaise:    b.PlatformFeePaise,
		SmallOrderFeePaise:  b.SmallOrderFeePaise,
		SurgeFeePaise:       b.SurgeFeePaise,
		TipPaise:            b.TipPaise,
		TaxPaise:            b.TaxPaise,
		GrandTotalPaise:     b.GrandTotalPaise,
		TaxLines:            []taxLineDTO{},
	}
	for _, tl := range b.TaxLines {
		dto.TaxLines = append(dto.TaxLines, taxLineDTO{
			Category: tl.Category, RateBP: tl.RateBP, TaxablePaise: tl.TaxablePaise, AmountPaise: tl.AmountPaise,
		})
	}
	return dto
}

func intOf(m map[string]any, key string) int64 {
	switch v := m[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

func nullStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func ptr(s string) *string { return &s }

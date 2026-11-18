// Package pricing is BiteOrbit's authoritative price engine. All math is
// integer paise; rounding is half-up applied per computed line. Frontends may
// preview prices but every authoritative amount is computed here and
// snapshotted onto the order at placement (ADR 006).
package pricing

import (
	"encoding/json"
	"fmt"
)

// Config mirrors pricing_rules.config (v1 demo schema).
type Config struct {
	DeliveryFeeBasePaise  int64 `json:"delivery_fee_base_paise"`
	DeliveryFeePerKmPaise int64 `json:"delivery_fee_per_km_paise"`
	DeliveryFeeMaxPaise   int64 `json:"delivery_fee_max_paise"`
	FreeDeliveryMinPaise  int64 `json:"free_delivery_min_paise"`
	PlatformFeeBP         int64 `json:"platform_fee_bp"`
	PlatformFeeCapPaise   int64 `json:"platform_fee_cap_paise"`
	SmallOrderFeePaise    int64 `json:"small_order_fee_paise"`
	SmallOrderBelowPaise  int64 `json:"small_order_below_paise"`
	Tax                   struct {
		Gst5BP   int64 `json:"gst_5_bp"`
		Gst12BP  int64 `json:"gst_12_bp"`
		ExemptBP int64 `json:"exempt_bp"`
	} `json:"tax"`
}

// ParseConfig decodes and sanity-checks a pricing rule config.
func ParseConfig(raw json.RawMessage) (*Config, error) {
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("pricing config: %w", err)
	}
	if c.DeliveryFeeBasePaise < 0 || c.DeliveryFeePerKmPaise < 0 ||
		c.PlatformFeeBP < 0 || c.PlatformFeeBP > 10000 || c.SmallOrderFeePaise < 0 {
		return nil, fmt.Errorf("pricing config has negative or out-of-range values")
	}
	return &c, nil
}

// LineInput is one cart line as priced by the engine. UnitPaise includes
// modifier deltas; TaxCategory selects the tax rate.
type LineInput struct {
	UnitPaise   int64
	Qty         int64
	TaxCategory string // gst_5 | gst_12 | exempt
}

// QuoteInput aggregates everything the engine needs.
type QuoteInput struct {
	Lines          []LineInput
	PackagingPaise int64
	DistanceKm     float64 // straight-line estimate, multiplied to a road-ish figure by the geo adapter
	TipPaise       int64
	CouponDiscount int64 // validated by the coupon service (0 when none)
}

// TaxLine documents one tax component for disclosure.
type TaxLine struct {
	Category     string `json:"category"`
	RateBP       int64  `json:"rate_bp"`
	TaxablePaise int64  `json:"taxable_paise"`
	AmountPaise  int64  `json:"amount_paise"`
}

// Breakdown is the full, disclosed price sheet.
type Breakdown struct {
	ItemsSubtotalPaise  int64     `json:"items_subtotal_paise"`
	ItemDiscountsPaise  int64     `json:"item_discounts_paise"`
	CouponDiscountPaise int64     `json:"coupon_discount_paise"`
	PackagingFeePaise   int64     `json:"packaging_fee_paise"`
	DeliveryFeePaise    int64     `json:"delivery_fee_paise"`
	PlatformFeePaise    int64     `json:"platform_fee_paise"`
	SmallOrderFeePaise  int64     `json:"small_order_fee_paise"`
	SurgeFeePaise       int64     `json:"surge_fee_paise"`
	TipPaise            int64     `json:"tip_paise"`
	TaxPaise            int64     `json:"tax_paise"`
	GrandTotalPaise     int64     `json:"grand_total_paise"`
	TaxLines            []TaxLine `json:"tax_lines"`
}

// roundHalfUpBP computes value*bp/10000 with half-up rounding, pure int math.
func roundHalfUpBP(value, bp int64) int64 {
	if value < 0 {
		return -roundHalfUpBP(-value, bp)
	}
	return (value*bp + 5000) / 10000
}

// Quote computes the authoritative breakdown.
func Quote(cfg *Config, in QuoteInput) Breakdown {
	subtotal := int64(0)
	for _, l := range in.Lines {
		subtotal += l.UnitPaise * l.Qty
	}

	// Coupon discount is distributed across lines proportionally (integer
	// allocation with the remainder on the first line) so per-line taxes are
	// computed on the discounted value, mirroring invoice practice.
	discount := in.CouponDiscount
	if discount > subtotal {
		discount = subtotal
	}

	tax := int64(0)
	taxLines := []TaxLine{}
	byCategory := map[string]*TaxLine{}
	allocated := int64(0)
	for i, l := range in.Lines {
		line := l.UnitPaise * l.Qty
		lineDiscount := int64(0)
		if subtotal > 0 {
			lineDiscount = discount * line / subtotal
		}
		if i == len(in.Lines)-1 {
			lineDiscount = discount - allocated // remainder lands on the last line
		}
		allocated += lineDiscount
		taxable := line - lineDiscount
		if taxable < 0 {
			taxable = 0
		}
		var rate int64
		switch l.TaxCategory {
		case "gst_5":
			rate = cfg.Tax.Gst5BP
		case "gst_12":
			rate = cfg.Tax.Gst12BP
		default:
			rate = cfg.Tax.ExemptBP
		}
		if rate == 0 {
			continue
		}
		tl, ok := byCategory[l.TaxCategory]
		if !ok {
			tl = &TaxLine{Category: l.TaxCategory, RateBP: rate}
			byCategory[l.TaxCategory] = tl
			taxLines = append(taxLines, *tl)
		}
		amount := roundHalfUpBP(taxable, rate)
		tl.TaxablePaise += taxable
		tl.AmountPaise += amount
		tax += amount
	}
	// copy mutated lines back into taxLines slice
	for i := range taxLines {
		if tl, ok := byCategory[taxLines[i].Category]; ok {
			taxLines[i] = *tl
		}
	}

	// fees
	delivery := int64(0)
	if subtotal < cfg.FreeDeliveryMinPaise {
		delivery = cfg.DeliveryFeeBasePaise + int64(cfg.DeliveryFeePerKmPaise*int64(in.DistanceKm*100)/100)
		if delivery > cfg.DeliveryFeeMaxPaise {
			delivery = cfg.DeliveryFeeMaxPaise
		}
	}
	platform := roundHalfUpBP(subtotal, cfg.PlatformFeeBP)
	if platform > cfg.PlatformFeeCapPaise {
		platform = cfg.PlatformFeeCapPaise
	}
	smallOrder := int64(0)
	if subtotal < cfg.SmallOrderBelowPaise {
		smallOrder = cfg.SmallOrderFeePaise
	}

	b := Breakdown{
		ItemsSubtotalPaise:  subtotal,
		ItemDiscountsPaise:  0,
		CouponDiscountPaise: discount,
		PackagingFeePaise:   in.PackagingPaise,
		DeliveryFeePaise:    delivery,
		PlatformFeePaise:    platform,
		SmallOrderFeePaise:  smallOrder,
		SurgeFeePaise:       0, // surge disabled in demo rule v1
		TipPaise:            in.TipPaise,
		TaxPaise:            tax,
		TaxLines:            taxLines,
	}
	b.GrandTotalPaise = subtotal - b.CouponDiscountPaise + b.PackagingFeePaise +
		b.DeliveryFeePaise + b.PlatformFeePaise + b.SmallOrderFeePaise +
		b.SurgeFeePaise + b.TipPaise + b.TaxPaise
	if b.GrandTotalPaise < 0 {
		b.GrandTotalPaise = 0
	}
	return b
}

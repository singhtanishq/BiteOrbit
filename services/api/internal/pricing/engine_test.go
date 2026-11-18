package pricing

import (
	"encoding/json"
	"testing"
)

func demoConfig(t *testing.T) *Config {
	t.Helper()
	raw := json.RawMessage(`{
		"delivery_fee_base_paise": 2900,
		"delivery_fee_per_km_paise": 900,
		"delivery_fee_max_paise": 9900,
		"free_delivery_min_paise": 59900,
		"platform_fee_bp": 400,
		"platform_fee_cap_paise": 2500,
		"small_order_fee_paise": 3000,
		"small_order_below_paise": 19900,
		"tax": {"gst_5_bp": 500, "gst_12_bp": 1200, "exempt_bp": 0}
	}`)
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("parse config: %v", err)
	}
	return cfg
}

func TestQuoteNoCouponDeliveryCharged(t *testing.T) {
	cfg := demoConfig(t)
	b := Quote(cfg, QuoteInput{
		Lines:          []LineInput{{UnitPaise: 24900, Qty: 1, TaxCategory: "gst_5"}},
		PackagingPaise: 2000,
		DistanceKm:     2.6,
		TipPaise:       2000,
	})
	if b.ItemsSubtotalPaise != 24900 {
		t.Fatalf("subtotal = %d", b.ItemsSubtotalPaise)
	}
	// 29 + 2.6*9 = 29+23.4 = 52.4 -> floor via int math on km*100: 29+900*260/100=29+2340=5240
	if b.DeliveryFeePaise != 5240 {
		t.Fatalf("delivery = %d, want 5240", b.DeliveryFeePaise)
	}
	// platform: 24900*400/10000 = 996
	if b.PlatformFeePaise != 996 {
		t.Fatalf("platform = %d", b.PlatformFeePaise)
	}
	// small order: 24900 >= 19900 → none
	if b.SmallOrderFeePaise != 0 {
		t.Fatalf("small order fee = %d", b.SmallOrderFeePaise)
	}
	// tax: (24900-2000? no discount) 24900*500/10000 half-up = 1245
	if b.TaxPaise != 1245 {
		t.Fatalf("tax = %d, want 1245", b.TaxPaise)
	}
	want := int64(24900 + 2000 + 5240 + 996 + 0 + 2000 + 1245)
	if b.GrandTotalPaise != want {
		t.Fatalf("grand total = %d, want %d (%+v)", b.GrandTotalPaise, want, b)
	}
}

func TestQuoteFreeDeliveryAndCap(t *testing.T) {
	cfg := demoConfig(t)
	// subtotal 599+ → free delivery; platform fee capped at 2500
	b := Quote(cfg, QuoteInput{
		Lines:      []LineInput{{UnitPaise: 32900, Qty: 3, TaxCategory: "gst_5"}},
		DistanceKm: 50, // would be huge without cap; free anyway
	})
	if b.DeliveryFeePaise != 0 {
		t.Fatalf("free delivery not applied: %d", b.DeliveryFeePaise)
	}
	if b.PlatformFeePaise != 2500 {
		t.Fatalf("platform cap not applied: %d", b.PlatformFeePaise)
	}
}

func TestQuoteSmallOrderFee(t *testing.T) {
	cfg := demoConfig(t)
	b := Quote(cfg, QuoteInput{
		Lines:      []LineInput{{UnitPaise: 8900, Qty: 1, TaxCategory: "gst_5"}},
		DistanceKm: 1,
	})
	if b.SmallOrderFeePaise != 3000 {
		t.Fatalf("small order fee = %d", b.SmallOrderFeePaise)
	}
}

func TestQuoteCouponDiscountTaxesOnDiscountedValue(t *testing.T) {
	cfg := demoConfig(t)
	// 2 lines, discount 5000 spread proportionally
	b := Quote(cfg, QuoteInput{
		Lines: []LineInput{
			{UnitPaise: 24900, Qty: 1, TaxCategory: "gst_5"},
			{UnitPaise: 25100, Qty: 1, TaxCategory: "gst_5"},
		},
		CouponDiscount: 5000,
		DistanceKm:     1,
	})
	if b.CouponDiscountPaise != 5000 {
		t.Fatalf("coupon = %d", b.CouponDiscountPaise)
	}
	// line discounts: 5000*24900/50000=2490; remainder 2510 on last line
	// tax: (24900-2490)*5% = 1120.5 → 1121; (25100-2510)*5% = 1129.5 → 1130; total 2251
	if b.TaxPaise != 2251 {
		t.Fatalf("tax on discounted = %d, want 2251 (lines %+v)", b.TaxPaise, b.TaxLines)
	}
	if len(b.TaxLines) != 1 || b.TaxLines[0].TaxablePaise != 45000 {
		t.Fatalf("tax line aggregation wrong: %+v", b.TaxLines)
	}
}

func TestQuoteMixedTaxCategories(t *testing.T) {
	cfg := demoConfig(t)
	b := Quote(cfg, QuoteInput{
		Lines: []LineInput{
			{UnitPaise: 10900, Qty: 1, TaxCategory: "gst_12"}, // 1308
			{UnitPaise: 14900, Qty: 1, TaxCategory: "gst_5"},  // 745
			{UnitPaise: 9900, Qty: 1, TaxCategory: "exempt"},  // 0
		},
		DistanceKm: 1,
	})
	if b.TaxPaise != 1308+745 {
		t.Fatalf("mixed tax = %d", b.TaxPaise)
	}
	if len(b.TaxLines) != 2 {
		t.Fatalf("expected 2 tax lines, got %+v", b.TaxLines)
	}
}

func TestQuoteDiscountCannotExceedSubtotal(t *testing.T) {
	cfg := demoConfig(t)
	b := Quote(cfg, QuoteInput{
		Lines:          []LineInput{{UnitPaise: 10000, Qty: 1, TaxCategory: "gst_5"}},
		CouponDiscount: 99999,
		DistanceKm:     1,
	})
	if b.CouponDiscountPaise != 10000 {
		t.Fatalf("discount must clamp to subtotal, got %d", b.CouponDiscountPaise)
	}
	if b.GrandTotalPaise < 0 {
		t.Fatalf("grand total negative: %d", b.GrandTotalPaise)
	}
}

func TestRoundHalfUp(t *testing.T) {
	cases := []struct {
		value, bp, want int64
	}{
		{1245, 500, 62}, // 62.25 → 62
		{1249, 500, 62}, // 62.45 → 62
		{1250, 500, 63}, // 62.5 → 63 (half up)
		{24900, 400, 996},
	}
	for _, c := range cases {
		if got := roundHalfUpBP(c.value, c.bp); got != c.want {
			t.Fatalf("roundHalfUpBP(%d,%d) = %d, want %d", c.value, c.bp, got, c.want)
		}
	}
}

func TestParseConfigRejectsGarbage(t *testing.T) {
	if _, err := ParseConfig(json.RawMessage(`{"platform_fee_bp": -5}`)); err == nil {
		t.Fatal("negative platform fee must be rejected")
	}
	if _, err := ParseConfig(json.RawMessage(`not json`)); err == nil {
		t.Fatal("invalid json must be rejected")
	}
}

package seed

// seedPlatform creates feature flags, pricing rules, commission rules.
func (s *seeder) seedPlatform() error {
	flags := []struct {
		key, desc string
		enabled   bool
	}{
		{"live_tracking_sse", "SSE-based live tracking (with polling fallback)", true},
		{"scheduled_orders", "Scheduled ordering slots", false},
		{"surge_pricing", "Peak-hour surge fee", false},
		{"wallet_ledger", "Wallet/credits (P2, ledger-backed)", false},
	}
	for _, f := range flags {
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO feature_flags (key, enabled, description) VALUES ($1,$2,$3)`,
			f.key, f.enabled, f.desc); err != nil {
			return err
		}
	}

	// Pricing rule v1: delivery fee by band, platform fee 4% capped ₹25,
	// small-order fee ₹30 below ₹199, no surge. Amounts in paise.
	pricingConfig := `{
		"delivery_fee_base_paise": 2900,
		"delivery_fee_per_km_paise": 900,
		"delivery_fee_max_paise": 9900,
		"free_delivery_min_paise": 59900,
		"platform_fee_bp": 400,
		"platform_fee_cap_paise": 2500,
		"small_order_fee_paise": 3000,
		"small_order_below_paise": 19900,
		"surge": {"enabled": false, "windows": []},
		"tax": {"gst_5_bp": 500, "gst_12_bp": 1200, "exempt_bp": 0},
		"packaging_source": "outlet"
	}`
	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO pricing_rules (version, name, effective_from, is_active, config)
		VALUES (1, 'demo-pricing-v1', now() - interval '60 days', true, $1::jsonb)`,
		pricingConfig); err != nil {
		return err
	}
	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO commission_rules (outlet_id, commission_bp, effective_from)
		VALUES (NULL, 1800, now() - interval '60 days')`); err != nil {
		return err
	}
	if _, err := s.tx.Exec(s.ctx, `
		INSERT INTO platform_settings (key, value) VALUES ('demo_city', '"Bitepur"'::jsonb)
		ON CONFLICT (key) DO NOTHING`); err != nil {
		return err
	}
	return nil
}

// seedCity creates the synthetic demo city of Bitepur with localities/zones.
func (s *seeder) seedCity() error {
	// Deterministic, plausible coordinates around a synthetic city centre.
	// (Bitepur does not exist; coordinates are synthetic and non-sensitive.)
	const centerLat, centerLng = 17.4126, 78.4632
	zones := []struct {
		name string
		dLat float64
		dLng float64
		rkm  float64
	}{
		{"Rosewood Hills", 0.036, -0.021, 3.2},
		{"Lakeview Enclave", -0.028, 0.018, 3.0},
		{"Charbagh Market", 0.012, 0.030, 2.8},
		{"Silicon Meadow", -0.041, -0.033, 3.5},
		{"Old Mill Quarter", 0.048, 0.008, 2.6},
		{"Harbour Line", -0.015, -0.042, 3.1},
		{"Greenfield Heights", 0.022, -0.047, 2.9},
		{"Cantonment South", 0.005, 0.052, 2.7},
	}
	s.zoneCenter = map[string][2]float64{}
	for _, z := range zones {
		id := s.mustInsert(`
			INSERT INTO service_zones (name, city, center_lat, center_lng, radius_km)
			VALUES ($1, 'Bitepur', $2, $3, $4) RETURNING id`,
			z.name, centerLat+z.dLat, centerLng+z.dLng, z.rkm)
		s.zoneIDs = append(s.zoneIDs, id)
		s.zoneName[id] = z.name
		s.zoneCenter[id] = [2]float64{centerLat + z.dLat, centerLng + z.dLng}
	}
	return nil
}

func (s *seeder) seedCuisines() error {
	cuisines := []string{
		"Biryani", "North Indian", "South Indian", "Chinese", "Pizza",
		"Burgers", "Rolls", "Chaat", "Mughlai", "Desserts",
		"Healthy", "Beverages", "Thalis", "Seafood", "Momos", "Breakfast",
	}
	for _, c := range cuisines {
		slug := ""
		for _, r := range c {
			if r == ' ' {
				slug += "-"
			} else if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				slug += string(lowerAscii(r))
			}
		}
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO cuisine_types (name, slug) VALUES ($1,$2) ON CONFLICT (name) DO NOTHING`,
			c, slug); err != nil {
			return err
		}
	}
	return nil
}

func lowerAscii(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

package seed

import (
	"fmt"
)

type outletSpec struct {
	name        string
	org         string // organisation display name
	cuisines    []string
	collections []string
	veg         bool
	priceBand   int
	description string
}

// All names are invented for BiteOrbit's synthetic demo and do not refer to
// real businesses.
var outletSpecs = []outletSpec{
	{"Biryani Bandit", "Bandit Kitchens", []string{"Biryani", "Mughlai", "North Indian"}, []string{"Trending", "Top rated"}, false, 3,
		"Slow-cooked dum biryanis and kebabs, cooked on order in sealed handis."},
	{"Tandoor Tales", "Tales Hospitality", []string{"North Indian", "Mughlai"}, []string{"Trending"}, false, 2,
		"Clay-oven classics, hearty curries and fresh breads."},
	{"Dosa Diner", "Diner South Co", []string{"South Indian", "Breakfast"}, []string{"Quick meals", "Veg friendly"}, true, 1,
		"Crisp dosas, fluffy idlis and filter coffee, all day."},
	{"Wok This Way", "Wok Works", []string{"Chinese"}, []string{"Trending", "Quick meals"}, false, 2,
		"Street-style wok tosses, dumplings and saucy noodles."},
	{"Pita Pan", "Wanderfood Group", []string{"Healthy", "Rolls"}, []string{"Healthy picks"}, false, 2,
		"Mediterranean bowls, pita wraps and protein salads."},
	{"Burger Bazaar", "Bazaar Eats", []string{"Burgers"}, []string{"Quick meals", "Budget bites"}, false, 1,
		"Smashed patties, house sauces, brioche buns."},
	{"Momo Matrix", "Matrix Foods", []string{"Momos", "Chinese"}, []string{"Budget bites", "Quick meals"}, false, 1,
		"Steamed, fried and tandoori momos with punchy chutneys."},
	{"Curry Carousel", "Carousel Kitchens", []string{"North Indian", "Thalis"}, []string{"Budget bites", "Veg friendly"}, true, 1,
		"Unlimited-ish happiness: thalis, dals and seasonal sabzis."},
	{"Roll Rebellion", "Rebellion Foods", []string{"Rolls", "Chinese"}, []string{"Trending", "Quick meals"}, false, 2,
		"Kathi rolls with bold fillings and flaky parathas."},
	{"Sweet Samsara", "Samsara Desserts", []string{"Desserts", "Beverages"}, []string{"Dessert runs"}, true, 2,
		"Modern Indian desserts, kulfis and shakes."},
	{"Grill Garrison", "Garrison Grill Co", []string{"North Indian", "Seafood"}, []string{"Top rated"}, false, 3,
		"Charcoal grills, coastal fish and smoky marinades."},
	{"Thali Trunk", "Trunk Kitchens", []string{"Thalis", "North Indian", "South Indian"}, []string{"Budget bites", "Veg friendly"}, true, 1,
		"Grand vegetarian thalis with rotating regional menus."},
	{"Chaat Chakra", "Chakra Streetco", []string{"Chaat", "Beverages"}, []string{"Budget bites", "Quick meals"}, true, 1,
		"Tangy, crunchy, spicy — street chaat done hygienically."},
	{"Slice Society", "Society Pizza Co", []string{"Pizza"}, []string{"Trending", "Top rated"}, false, 2,
		"Sourdough pizzas with a wood-fired soul."},
	{"Kathi Kompass", "Kompass Foods", []string{"Rolls", "Mughlai"}, []string{"Quick meals"}, false, 2,
		"Kolkata-style kathi rolls and chaap specials."},
	{"Sundae Signal", "Signal Sweets", []string{"Desserts", "Beverages"}, []string{"Dessert runs", "Budget bites"}, true, 1,
		"Sundaes, brownies and cold coffees till late."},
	{"Pulao Planet", "Planet Foods", []string{"Biryani", "North Indian"}, []string{"Top rated", "Family feasts"}, false, 3,
		"Region-inspired pulaos, biryanis and side feasts."},
	{"Idli Isle", "Isle Breakfast Co", []string{"South Indian", "Breakfast", "Healthy"}, []string{"Veg friendly", "Healthy picks"}, true, 1,
		"Batter ground fresh daily; millet idlis and podi everything."},
}

// seedRestaurants creates organisations, outlets, staff, hours, service areas,
// menus with categories/items/modifiers.
func (s *seeder) seedRestaurants() error {
	// organisations keyed by display name; owners keyed the same way
	orgIDs := map[string]string{}
	orgOwners := map[string]string{}

	for i, spec := range outletSpecs {
		orgID, ok := orgIDs[spec.org]
		var ownerID string
		if !ok {
			ownerEmail := fmt.Sprintf("owner%02d@demo.biteorbit.local", len(orgIDs)+1)
			ownerName := fmt.Sprintf("%s Owner", spec.org)
			ownerID = s.mustInsert(`
				INSERT INTO users (email, phone, name) VALUES ($1,$2,$3) RETURNING id`,
				ownerEmail, fmt.Sprintf("+9199200%05d", len(orgIDs)+1), ownerName)
			s.grant(ownerID, "restaurant_owner")
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO credentials (user_id, password_hash) VALUES ($1,$2)`, ownerID, s.hash); err != nil {
				return err
			}
			orgID = s.mustInsert(`
				INSERT INTO restaurant_organisations (legal_name, display_name, owner_user_id, contact_email, contact_phone, status)
				VALUES ($1,$2,$3,$4,$5,'approved') RETURNING id`,
				spec.org+" Pvt Ltd (demo)", spec.org, ownerID,
				ownerEmail, fmt.Sprintf("+9199300%05d", len(orgIDs)+1))
			orgIDs[spec.org] = orgID
			orgOwners[spec.org] = ownerID

			// An approved application with review history.
			appID := s.mustInsert(`
				INSERT INTO restaurant_applications (organisation_id, status, applicant_note, reviewer_id, decision_note, decided_at)
				VALUES ($1,'approved',$2,$3,'Documents verified in demo review (simulation).', now() - interval '45 days')
				RETURNING id`,
				orgID, "We would love to serve the Bitepur demo city.", s.adminID)
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO application_events (application_id, from_status, to_status, actor_id, note)
				VALUES ($1,'submitted','under_review',$2,'Demo review started.'),
				       ($1,'under_review','approved',$2,'Demo approval (simulation).')`,
				appID, s.adminID); err != nil {
				return err
			}
		}
		ownerID = orgOwners[spec.org]

		zone := s.zoneIDs[i%len(s.zoneIDs)]
		zc := s.zoneCenter[zone]
		lat := r2(zc[0] + jitter(s.rng, 0.015))
		lng := r2(zc[1] + jitter(s.rng, 0.015))
		outletID := s.mustInsert(`
			INSERT INTO outlets
			(organisation_id, name, description, phone, address_line1, address_line2, locality, city, state, postal_code, lat, lng, status, is_pure_veg, min_order_paise, packaging_fee_paise, prep_minutes_min, prep_minutes_max, price_band)
			VALUES ($1,$2,$3,$4,$5,NULL,$6,'Bitepur','Telengana Heights',$7,$8,$9,'active',$10,$11,$12,$13,$14,$15)
			RETURNING id`,
			orgID, spec.name, spec.description,
			fmt.Sprintf("+9199400%05d", i+1),
			fmt.Sprintf("%d, %s Main Road", 10+i*3, s.zoneName[zone]),
			s.zoneName[zone],
			fmt.Sprintf("560%03d", s.rng.Intn(900)),
			lat, lng, spec.veg,
			pickInt(s.rng, 0, 14900, 19900, 24900),
			pickInt(s.rng, 1500, 2000, 2500),
			pickInt(s.rng, 12, 15, 18),
			pickInt(s.rng, 25, 30, 35),
			spec.priceBand)
		s.outletIDs = append(s.outletIDs, outletID)
		s.outletOrg[outletID] = orgID
		s.outletName[outletID] = spec.name

		// Owner is staff on every outlet of the org.
		if ownerID != "" {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO outlet_staff (outlet_id, user_id, role, granted_by)
				VALUES ($1,$2,'owner',$3) ON CONFLICT DO NOTHING`,
				outletID, ownerID, ownerID); err != nil {
				return err
			}
		}

		// cuisines + collections + hours + service areas
		for _, c := range spec.cuisines {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO outlet_cuisines (outlet_id, cuisine_id)
				SELECT $1, id FROM cuisine_types WHERE name = $2
				ON CONFLICT DO NOTHING`, outletID, c); err != nil {
				return err
			}
		}
		for _, col := range spec.collections {
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO outlet_collections (collection, outlet_id) VALUES ($1,$2)
				ON CONFLICT DO NOTHING`, col, outletID); err != nil {
				return err
			}
		}
		for wd := 0; wd < 7; wd++ {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO outlet_hours (outlet_id, weekday, opens_at, closes_at, is_closed)
				VALUES ($1,$2,$3,$4,false)`,
				outletID, wd, pick(s.rng, "09:00", "10:00", "11:00"), pick(s.rng, "22:00", "23:00", "23:30")); err != nil {
				return err
			}
		}
		for _, z := range s.zoneIDs {
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO outlet_service_areas (outlet_id, zone_id) VALUES ($1,$2)
				ON CONFLICT DO NOTHING`, outletID, z); err != nil {
				return err
			}
		}
		// A couple of upcoming closures.
		if i%6 == 0 {
			if _, err := s.tx.Exec(s.ctx, `
				INSERT INTO outlet_holidays (outlet_id, holiday_date, reason)
				VALUES ($1, current_date + 3, 'Staff training (demo)')`, outletID); err != nil {
				return err
			}
		}

		// manager + staff accounts for portal demos
		if i < 3 {
			mEmail := fmt.Sprintf("manager%02d@demo.biteorbit.local", i+1)
			mID := s.mustInsert(`
				INSERT INTO users (email, phone, name) VALUES ($1,$2,$3) RETURNING id`,
				mEmail, fmt.Sprintf("+9199500%05d", i+1), fmt.Sprintf("%s Manager", spec.name))
			s.grant(mID, "restaurant_manager")
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO credentials (user_id, password_hash) VALUES ($1,$2)`, mID, s.hash); err != nil {
				return err
			}
			if _, err := s.tx.Exec(s.ctx,
				`INSERT INTO outlet_staff (outlet_id, user_id, role, granted_by)
				VALUES ($1,$2,'manager',$3)`, outletID, mID, s.adminID); err != nil {
				return err
			}
		}
	}

	for _, oID := range s.outletIDs {
		if err := s.seedMenuForOutlet(oID); err != nil {
			return err
		}
	}
	return nil
}

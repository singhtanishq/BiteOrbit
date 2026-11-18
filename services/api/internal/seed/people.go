package seed

import (
	"fmt"
)

// seedPeople creates the synthetic admin, support, customer and rider accounts.
// All accounts share DemoPassword and exist only in local demo data.
func (s *seeder) seedPeople() error {
	// --- platform staff -----------------------------------------------------
	s.adminID = s.mustInsert(`
		INSERT INTO users (email, phone, name) VALUES ('admin@demo.biteorbit.local','+919900000001','Aarav Mehta (Admin)') RETURNING id`)
	s.grant(s.adminID, "admin")
	s.opsManagerID = s.mustInsert(`
		INSERT INTO users (email, phone, name) VALUES ('ops@demo.biteorbit.local','+919900000002','Diya Kapoor (Ops)') RETURNING id`)
	s.grant(s.opsManagerID, "ops_manager")
	s.supportID = s.mustInsert(`
		INSERT INTO users (email, phone, name) VALUES ('support@demo.biteorbit.local','+919900000003','Kabir Rao (Support)') RETURNING id`)
	s.grant(s.supportID, "support_agent")

	for _, uid := range []string{s.adminID, s.opsManagerID, s.supportID} {
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO credentials (user_id, password_hash) VALUES ($1,$2)`, uid, s.hash); err != nil {
			return err
		}
	}

	// --- customers ----------------------------------------------------------
	customers := []struct{ email, name, phone string }{
		{"customer@demo.biteorbit.local", "Ishaan Verma", "+919910000001"},
		{"customer2@demo.biteorbit.local", "Ananya Shetty", "+919910000002"},
		{"customer3@demo.biteorbit.local", "Rehaan Malik", "+919910000003"},
		{"customer4@demo.biteorbit.local", "Meera Iyer", "+919910000004"},
		{"customer5@demo.biteorbit.local", "Zoya Khan", "+919910000005"},
		{"customer6@demo.biteorbit.local", "Arjun Nair", "+919910000006"},
		{"customer7@demo.biteorbit.local", "Tara Bose", "+919910000007"},
		{"customer8@demo.biteorbit.local", "Vivaan Joshi", "+919910000008"},
	}
	for _, c := range customers {
		uid := s.mustInsert(`
			INSERT INTO users (email, phone, name) VALUES ($1,$2,$3) RETURNING id`,
			c.email, c.phone, c.name)
		s.grant(uid, "customer")
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO credentials (user_id, password_hash) VALUES ($1,$2)`, uid, s.hash); err != nil {
			return err
		}
		s.customerIDs = append(s.customerIDs, uid)

		// Two addresses per customer in different zones.
		z1 := s.zoneIDs[s.rng.Intn(len(s.zoneIDs))]
		z2 := s.zoneIDs[s.rng.Intn(len(s.zoneIDs))]
		zc1, zc2 := s.zoneCenter[z1], s.zoneCenter[z2]
		postal1 := fmt.Sprintf("560%03d", s.rng.Intn(900))
		postal2 := fmt.Sprintf("560%03d", s.rng.Intn(900))
		if _, err := s.tx.Exec(s.ctx, `
			INSERT INTO customer_addresses
			(user_id, label, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, delivery_instructions, is_default)
			VALUES ($1,'home',$2,$3,$4,$5,$6,$7,'Bitepur','Telengana Heights',$8,$9,$10,'Ring the bell twice; leave with security if away.',true)`,
			uid, c.name, c.phone,
			fmt.Sprintf("%d, %s Residency", 10+s.rng.Intn(89), s.zoneName[z1]),
			"Block "+string(rune('A'+s.rng.Intn(6))),
			pick(s.rng, "Near the park gate", "Opposite community hall", "Behind the creche"),
			s.zoneName[z1], postal1, zc1[0], zc1[1]); err != nil {
			return err
		}
		if _, err := s.tx.Exec(s.ctx, `
			INSERT INTO customer_addresses
			(user_id, label, recipient_name, recipient_phone, line1, line2, landmark, locality, city, state, postal_code, lat, lng, delivery_instructions, is_default)
			VALUES ($1,'work',$2,$3,$4,NULL,$5,$6,'Bitepur','Telengana Heights',$7,$8,$9,'Reception desk, 3rd floor.',false)`,
			uid, c.name, c.phone,
			fmt.Sprintf("Tower %d, %s Tech Park", 1+s.rng.Intn(5), s.zoneName[z2]),
			pick(s.rng, "Main gate", "Service lift", "Lobby B"),
			s.zoneName[z2], postal2, zc2[0], zc2[1]); err != nil {
			return err
		}
		if _, err := s.tx.Exec(s.ctx, `
			INSERT INTO customer_preferences (user_id, is_veg_pref, preferred_cuisines)
			VALUES ($1, $2, $3)`,
			uid, s.rng.Intn(3) == 0, []string{"Biryani", "Chinese"}); err != nil {
			return err
		}
	}

	// --- riders ---------------------------------------------------------------
	riders := []struct{ email, name, vehicle, vnumber string }{
		{"rider@demo.biteorbit.local", "Sanjay Patil", "bike", "MH12DQ1101"},
		{"rider2@demo.biteorbit.local", "Farhan Sheikh", "scooter", "TS09EF2202"},
		{"rider3@demo.biteorbit.local", "Manoj Yadav", "bike", "KA05GH3303"},
		{"rider4@demo.biteorbit.local", "Deepa Kulkarni", "ev", "TS11JK4404"},
		{"rider5@demo.biteorbit.local", "Imran Qureshi", "bike", "MH14LM5505"},
		{"rider6@demo.biteorbit.local", "Ritu Chawla", "scooter", "DL09NP6606"},
	}
	for i, r := range riders {
		uid := s.mustInsert(`
			INSERT INTO users (email, phone, name) VALUES ($1,$2,$3) RETURNING id`,
			r.email, fmt.Sprintf("+91992%07d", i+1), r.name)
		s.grant(uid, "rider")
		if _, err := s.tx.Exec(s.ctx,
			`INSERT INTO credentials (user_id, password_hash) VALUES ($1,$2)`, uid, s.hash); err != nil {
			return err
		}
		zone := s.zoneIDs[i%len(s.zoneIDs)]
		zc := s.zoneCenter[zone]
		if _, err := s.tx.Exec(s.ctx, `
			INSERT INTO delivery_partners
			(user_id, status, availability, last_online_at, vehicle_type, vehicle_number, base_zone_id, current_lat, current_lng)
			VALUES ($1, 'approved', $2, now(), $3, $4, $5, $6, $7)`,
			uid, pickWeighted(s.rng, 0.75, "online", "offline"),
			r.vehicle, r.vnumber, zone, zc[0]+jitter(s.rng, 0.01), zc[1]+jitter(s.rng, 0.01)); err != nil {
			return err
		}
	}
	return nil
}

func (s *seeder) grant(userID, role string) {
	if _, err := s.tx.Exec(s.ctx,
		`INSERT INTO user_roles (user_id, role) VALUES ($1,$2) ON CONFLICT DO NOTHING`,
		userID, role); err != nil {
		panic(err)
	}
}

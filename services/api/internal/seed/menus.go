package seed

import (
	"fmt"
	"math/rand"
)

// recipe is a deterministic menu template. prices are paise.
type recipe struct {
	category string
	items    []recipeItem
}

type recipeItem struct {
	name        string
	desc        string
	price       int
	veg         bool
	taxCategory string // gst_5 | gst_12 | exempt
	tags        []string
	allergens   string
	serving     string
	bestseller  bool
	featured    bool
	modifiers   []modSpec
}

type modSpec struct {
	group    string
	required bool
	min      int
	max      int
	options  []modOption
}

type modOption struct {
	name  string
	delta int
}

// it is a compact recipeItem constructor.
//
//nolint:revive // many params keep the menu tables readable
func it(name, desc string, price int, veg bool, tax string, tags []string,
	allergens, serving string, bestseller, featured bool, mods ...modSpec) recipeItem {
	return recipeItem{
		name: name, desc: desc, price: price, veg: veg, taxCategory: tax,
		tags: tags, allergens: allergens, serving: serving,
		bestseller: bestseller, featured: featured, modifiers: mods,
	}
}

var commonSizeMods = modSpec{
	group: "Portion", required: true, min: 1, max: 1,
	options: []modOption{{"Regular", 0}, {"Large", 8000}},
}

var spiceMod = modSpec{
	group: "Spice level", required: true, min: 1, max: 1,
	options: []modOption{{"Mild", 0}, {"Medium", 0}, {"Spicy", 0}},
}

var raitaMod = modSpec{
	group: "Add-ons", required: false, min: 0, max: 2,
	options: []modOption{{"Extra raita", 3000}, {"Salan pack", 2500}},
}

var drinkMod = modSpec{
	group: "Add-ons", required: false, min: 0, max: 1,
	options: []modOption{{"Coke (330 ml)", 6000}, {"Masala chaas", 4500}},
}

// menuTemplates maps cuisine to menu structures. Synthetic names only.
var menuTemplates = map[string][]recipe{
	"Biryani": {
		{"Signature Biryani", []recipeItem{
			it("Dum Handi Chicken Biryani", "Sealed handi, saffron, slow dum", 32900, false, "gst_5", []string{"biryani"}, "", "Serves 1", true, true, spiceMod, raitaMod),
			it("Mutton Dum Biryani", "Long-grain rice, tender mutton", 44900, false, "gst_5", []string{"biryani"}, "", "Serves 1", true, false, spiceMod, raitaMod),
			it("Veg Subz Biryani", "Garden vegetables, dum-cooked", 24900, true, "gst_5", []string{"biryani", "veg"}, "", "Serves 1", false, true, spiceMod, raitaMod),
			it("Paneer Tikka Biryani", "Charred paneer, mint rice", 28900, true, "gst_5", []string{"biryani", "veg"}, "Dairy", "Serves 1", false, false, spiceMod),
			it("Egg Biryani", "Boiled eggs, toasted spices", 21900, false, "gst_5", []string{"biryani", "egg"}, "Egg", "Serves 1", false, false, spiceMod),
		}},
		{"Sides & Raita", []recipeItem{
			it("Mirchi ka Salan", "Classic biryani companion", 8900, true, "gst_5", nil, "", "150 ml", false, false),
			it("Burani Raita", "Garlic yogurt", 5900, true, "gst_5", nil, "Dairy", "150 ml", false, false),
			it("Double ka Meetha", "Bread pudding, saffron milk", 12900, true, "gst_5", nil, "Dairy, gluten", "1 bowl", false, true),
		}},
	},
	"South Indian": {
		{"Dosas", []recipeItem{
			it("Ghee Roast Dosa", "Crisp, cast-iron, pure ghee", 14900, true, "gst_5", nil, "Dairy", "1 dosa + chutneys", true, true),
			it("Masala Dosa", "Potato filling, three chutneys", 11900, true, "gst_5", nil, "", "1 dosa", true, false),
			it("Mysore Masala Dosa", "Red chutney, extra crunch", 13900, true, "gst_5", nil, "", "1 dosa", false, true),
			it("Rava Onion Dosa", "Semolina lace, onions", 12900, true, "gst_5", nil, "", "1 dosa", false, false),
		}},
		{"Idli & Vada", []recipeItem{
			it("Idli Sambar (3 pc)", "Steamed rice cakes, sambar", 8900, true, "gst_5", nil, "", "3 idlis", true, false),
			it("Millet Idli Plate", "Foxtail millet, podi sprinkle", 12900, true, "gst_5", []string{"millet"}, "", "3 idlis", false, true),
			it("Medu Vada (2 pc)", "Crisp outside, fluffy inside", 7900, true, "gst_5", nil, "", "2 vadas", false, false),
		}},
		{"Beverages", []recipeItem{
			it("Filter Coffee", "Chicory blend, davara tumbler", 5900, true, "gst_5", nil, "Dairy", "150 ml", true, false),
			it("Rose Milk", "Chilled, nostalgic", 6900, true, "gst_5", nil, "Dairy", "200 ml", false, false),
		}},
	},
	"Chinese": {
		{"Wok Tossed", []recipeItem{
			it("Hakka Noodles", "Julienne veg, soy toss", 15900, true, "gst_5", nil, "Gluten, soy", "Serves 1", true, true, spiceMod),
			it("Chilli Garlic Noodles", "Fiery, garlic-forward", 16900, true, "gst_5", nil, "Gluten, soy", "Serves 1", false, false, spiceMod),
			it("Chicken Schezwan Noodles", "House schezwan paste", 21900, false, "gst_5", nil, "Gluten, soy", "Serves 1", true, false, spiceMod),
			it("Burnt Garlic Rice", "Crisp garlic chips", 14900, true, "gst_5", nil, "", "Serves 1", false, false),
		}},
		{"Starters", []recipeItem{
			it("Veg Spring Rolls (4 pc)", "Glass noodles, sweet chilli dip", 12900, true, "gst_5", nil, "Gluten", "4 rolls", false, false),
			it("Chicken Momos (8 pc)", "Steamed, ginger-soy", 17900, false, "gst_5", nil, "Gluten", "8 pcs", true, true, spiceMod),
		}},
	},
	"Pizza": {
		{"Wood-fired Pizzas", []recipeItem{
			it("Margherita Verace", "San Marzano, fior di latte", 24900, true, "gst_5", nil, "Dairy, gluten", "9 inch", true, true, commonSizeMods),
			it("Peri Peri Paneer", "Peri sauce, charred peppers", 31900, true, "gst_5", nil, "Dairy, gluten", "9 inch", true, false, commonSizeMods),
			it("Smoked Chicken Pepperoni", "House-cured pepperoni", 38900, false, "gst_5", nil, "Dairy, gluten", "9 inch", true, true, commonSizeMods),
			it("Truffle Mushroom", "Wild mushrooms, truffle oil", 36900, true, "gst_5", nil, "Dairy, gluten", "9 inch", false, true, commonSizeMods),
		}},
		{"Garlic Sides", []recipeItem{
			it("Cheesy Garlic Bread", "Mozzarella pull", 13900, true, "gst_5", nil, "Dairy, gluten", "6 sticks", false, false),
			it("Peri Fries", "Dusted, house mayo", 10900, true, "gst_12", nil, "", "Serves 1", false, false),
		}},
	},
	"Burgers": {
		{"Smash Burgers", []recipeItem{
			it("Classic Smash", "Double patty, house sauce", 18900, false, "gst_12", nil, "Gluten", "1 burger", true, true),
			it("Nashville Hot Chicken", "Buttermilk brined, cayenne glaze", 20900, false, "gst_12", nil, "Gluten", "1 burger", true, false),
			it("Mushroom Melt (Veg)", "Portobello, cheese sauce", 17900, true, "gst_12", nil, "Dairy, gluten", "1 burger", false, true),
			it("Bazaar Bean Burger", "Three-bean patty, smoked mayo", 15900, true, "gst_12", nil, "Gluten", "1 burger", false, false),
		}},
		{"Fries & Shakes", []recipeItem{
			it("Bazaar Fries", "Peri-dusted, cheese drizzle", 9900, true, "gst_12", nil, "Dairy", "Serves 1", false, false),
			it("Salted Caramel Shake", "Hand-spun", 13900, true, "gst_5", nil, "Dairy", "300 ml", false, true),
		}},
	},
	"Rolls": {
		{"Kathi Rolls", []recipeItem{
			it("Chicken Kathi Roll", "Egg-washed paratha, tangy onions", 16900, false, "gst_5", nil, "Gluten, egg", "1 roll", true, true, spiceMod),
			it("Paneer Tikka Roll", "Charred paneer, mint mayo", 14900, true, "gst_5", nil, "Dairy, gluten", "1 roll", true, false, spiceMod),
			it("Mutton Chaap Roll", "Slow-cooked chaap", 19900, false, "gst_5", nil, "Gluten", "1 roll", false, false),
			it("Aloo Kabab Roll", "Spiced potato kabab", 11900, true, "gst_5", nil, "Gluten", "1 roll", false, false),
		}},
	},
	"Chaat": {
		{"Street Classics", []recipeItem{
			it("Pani Puri (6 pc)", "Five waters, spiced mash", 6900, true, "gst_5", nil, "", "6 pcs", true, true),
			it("Dahi Puri (6 pc)", "Yogurt, chutneys, sev", 8900, true, "gst_5", nil, "Dairy", "6 pcs", false, false),
			it("Samosa Chaat", "Crushed samosa, chole, dahi", 9900, true, "gst_5", nil, "Dairy, gluten", "1 plate", true, false),
			it("Bhel Puri", "Puffed rice, tangy toss", 7900, true, "gst_5", nil, "", "1 plate", false, true),
		}},
	},
	"Mughlai": {
		{"Curries", []recipeItem{
			it("Butter Chicken", "Tandoori chicken, tomato-makhani", 32900, false, "gst_5", nil, "Dairy", "Serves 2", true, true, spiceMod),
			it("Galouti Kebab (4 pc)", "Melt-in-mouth mince patties", 28900, false, "gst_5", nil, "", "4 pcs", true, false),
			it("Dal Makhani", "48-hour simmered", 19900, true, "gst_5", nil, "Dairy", "Serves 2", true, true),
			it("Paneer Lababdar", "Silky tomato, fenugreek", 24900, true, "gst_5", nil, "Dairy", "Serves 2", false, false, spiceMod),
		}},
		{"Breads", []recipeItem{
			it("Butter Naan", "Tandoor-blistered", 5900, true, "gst_5", nil, "Dairy, gluten", "1 pc", false, false),
			it("Laccha Paratha", "Layered, crisp", 6900, true, "gst_5", nil, "Dairy, gluten", "1 pc", false, false),
		}},
	},
	"Desserts": {
		{"Desserts", []recipeItem{
			it("Biscoff Kulfi Falooda", "Kulfi, falooda, biscuit crumb", 15900, true, "gst_5", nil, "Dairy, gluten", "1 glass", true, true),
			it("Warm Brownie Sundae", "Fudge brownie, vanilla scoop", 13900, true, "gst_12", nil, "Dairy, gluten, egg", "1 plate", true, false),
			it("Kesar Pista Kulfi (2 pc)", "Slow-set, saffron", 9900, true, "gst_5", nil, "Dairy", "2 pcs", false, false),
		}},
		{"Shakes & Coffee", []recipeItem{
			it("Cold Coffee Frappe", "Double shot, ice blended", 12900, true, "gst_5", nil, "Dairy", "350 ml", false, false),
			it("Mango Lassi (seasonal)", "Thick, pulpy", 10900, true, "gst_5", nil, "Dairy", "300 ml", false, true),
		}},
	},
	"Healthy": {
		{"Protein Bowls", []recipeItem{
			it("Grilled Chicken Quinoa Bowl", "40g protein, tahini drizzle", 31900, false, "gst_12", []string{"high-protein"}, "", "Serves 1", true, true),
			it("Paneer Caesar Bowl", "Romaine, wholegrain croutons", 26900, true, "gst_12", nil, "Dairy, gluten", "Serves 1", true, false),
			it("Falafel Hummus Bowl", "Baked falafel, pickles", 24900, true, "gst_12", []string{"vegan"}, "Gluten (pita)", "Serves 1", false, true),
		}},
		{"Wraps", []recipeItem{
			it("Chicken Peri Wrap", "Wholewheat wrap", 22900, false, "gst_5", nil, "Gluten", "1 wrap", false, false),
			it("Tofu Peanut Wrap", "Crunchy veg, peanut sauce", 19900, true, "gst_5", []string{"vegan"}, "Peanuts, gluten", "1 wrap", false, false),
		}},
	},
	"Thalis": {
		{"Thalis", []recipeItem{
			it("Grand Veg Thali", "3 sabzis, dal, rice, 2 breads, dessert", 24900, true, "gst_5", []string{"unlimited-rotis"}, "Dairy, gluten", "Full thali", true, true),
			it("South Indian Meals", "Sambar, rasam, poriyal, curd", 19900, true, "gst_5", nil, "Dairy", "Full meals", true, false),
			it("Non-Veg Special Thali", "Chicken curry, egg, fish fry (alt days)", 32900, false, "gst_5", nil, "", "Full thali", false, true),
		}},
	},
	"Momos": {
		{"Momos", []recipeItem{
			it("Steamed Veg Momos (8 pc)", "Cabbage-carrot filling", 9900, true, "gst_5", nil, "Gluten", "8 pcs", true, true, spiceMod),
			it("Steamed Chicken Momos (8 pc)", "Juicy chicken mince", 11900, false, "gst_5", nil, "Gluten", "8 pcs", true, false, spiceMod),
			it("Tandoori Paneer Momos (8 pc)", "Char-grilled, mint cream", 14900, true, "gst_12", nil, "Dairy, gluten", "8 pcs", false, true),
		}},
	},
	"Seafood": {
		{"Coastal Grills", []recipeItem{
			it("Tandoori Pomfret", "Whole fish, ajwain marinade", 44900, false, "gst_5", nil, "Fish", "1 fish", true, true),
			it("Prawn Koliwada", "Crisp-fried, curry leaf", 36900, false, "gst_5", nil, "Shellfish", "Serves 2", false, false, spiceMod),
			it("Meen Pollichathu", "Kerala style, banana leaf", 42900, false, "gst_5", nil, "Fish", "1 portion", false, true),
		}},
	},
	"North Indian": {
		{"Mains", []recipeItem{
			it("Dal Tadka", "Ghee tempering", 15900, true, "gst_5", nil, "Dairy", "Serves 2", true, false),
			it("Chicken Curry (Home Style)", "Onion-tomato masala", 28900, false, "gst_5", nil, "", "Serves 2", true, true, spiceMod),
			it("Malai Kofta", "Cottage dumplings, cashew gravy", 22900, true, "gst_5", nil, "Dairy, nuts", "Serves 2", false, true),
			it("Jeera Rice", "Cumin-tempered basmati", 11900, true, "gst_5", nil, "", "Serves 2", false, false),
		}},
	},
	"Beverages": {
		{"Coolers", []recipeItem{
			it("Masala Nimbu Soda", "Sweet-salt-spice", 6900, true, "gst_12", nil, "", "300 ml", true, false),
			it("Jaljeera Fizz", "Cumin-mint punch", 6900, true, "gst_12", nil, "", "300 ml", false, true),
			it("Fresh Lime Water", "Sweet or salt", 4900, true, "gst_12", nil, "", "300 ml", false, false),
		}},
	},
	"Breakfast": {
		{"All-day Breakfast", []recipeItem{
			it("Poha with Sev", "Kanda poha, lemon", 7900, true, "gst_5", nil, "", "Serves 1", true, false),
			it("Upma Kichdi", "Veg-loaded, ghee", 8900, true, "gst_5", nil, "Dairy", "Serves 1", false, true),
			it("Masala Omelette + Toast", "Two eggs, onion-chilli", 11900, false, "gst_12", nil, "Egg, gluten", "1 plate", false, false),
		}},
	},
}

// menuForCuisines assembles an outlet menu from its cuisine templates.
func menuForCuisines(cuisines []string) []recipe {
	var out []recipe
	seen := map[string]bool{}
	for _, c := range cuisines {
		if tpl, ok := menuTemplates[c]; ok && !seen[c] {
			seen[c] = true
			out = append(out, tpl...)
		}
	}
	if len(out) == 0 {
		out = append(out, menuTemplates["North Indian"]...)
	}
	return out
}

// seedMenuForOutlet builds categories, items, modifiers for one outlet.
func (s *seeder) seedMenuForOutlet(outletID string) error {
	recipes := menuForCuisines(s.outletCuisines(outletID))
	order := 0
	for _, rec := range recipes {
		catID := s.mustInsert(`
			INSERT INTO menu_categories (outlet_id, name, display_order) VALUES ($1,$2,$3) RETURNING id`,
			outletID, rec.category, order)
		order++
		for _, item := range rec.items {
			tags := item.tags
			if tags == nil {
				tags = []string{}
			}
			itemID := s.mustInsert(`
				INSERT INTO menu_items
				(outlet_id, category_id, name, description, price_paise, tax_category, is_veg, dietary_tags, allergen_notes, serving_info, is_featured, is_bestseller, is_available, display_order)
				VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,true,$13)
				RETURNING id`,
				outletID, catID, item.name, item.desc, item.price, item.taxCategory,
				item.veg, tags, item.allergens, item.serving, item.featured, item.bestseller, order)
			s.itemIDs[outletID] = append(s.itemIDs[outletID], itemID)

			// Attach modifiers deterministically by item name.
			h := hashStr(item.name)
			mods := item.modifiers
			if len(mods) == 0 && h%3 == 0 {
				mods = append(mods, spiceMod)
			}
			if h%5 == 0 {
				mods = append(mods, drinkMod)
			}
			for mi, m := range mods {
				groupID := s.mustInsert(`
					INSERT INTO modifier_groups (outlet_id, name, is_required, min_select, max_select, display_order)
					VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`,
					outletID, m.group, m.required, m.min, m.max, mi)
				if _, err := s.tx.Exec(s.ctx,
					`INSERT INTO item_modifier_groups (item_id, group_id, display_order) VALUES ($1,$2,$3)`,
					itemID, groupID, mi); err != nil {
					return err
				}
				for oi, o := range m.options {
					if _, err := s.tx.Exec(s.ctx, `
						INSERT INTO modifier_options (group_id, name, price_delta_paise, is_default, is_available)
						VALUES ($1,$2,$3,$4,true)`, groupID, o.name, o.delta, oi == 0); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (s *seeder) outletCuisines(outletID string) []string {
	rows, err := s.tx.Query(s.ctx, `
		SELECT ct.name FROM outlet_cuisines oc
		JOIN cuisine_types ct ON ct.id = oc.cuisine_id
		WHERE oc.outlet_id = $1 ORDER BY ct.name`, outletID)
	if err != nil {
		return []string{"North Indian"}
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

func hashStr(s string) int {
	h := 5381
	for _, c := range s {
		h = h*33 + int(c)
	}
	if h < 0 {
		h = -h
	}
	return h
}

var _ = fmt.Sprintf
var _ = rand.Intn

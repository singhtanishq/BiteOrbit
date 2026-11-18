-- BiteOrbit initial schema.
CREATE EXTENSION IF NOT EXISTS citext;

-- Conventions:
--   * money: BIGINT paise (1 INR = 100 paise), currency code INR on financial rows
--   * timestamps: timestamptz (UTC); demo timezone Asia/Kolkata rendered client-side
--   * enums: text + CHECK constraints (simpler migrations than native enums)
--   * soft delete: deleted_at where business records need archival without
--     breaking unique constraints

-- ===========================================================================
-- IDENTITY & ACCESS
-- ===========================================================================

CREATE TABLE users (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email       citext NOT NULL UNIQUE,
    phone       varchar(20),
    name        text NOT NULL,
    status      text NOT NULL DEFAULT 'active'
                CHECK (status IN ('active','suspended','deletion_requested','deleted')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    deleted_at  timestamptz
);

CREATE TABLE user_roles (
    user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role     text NOT NULL CHECK (role IN
             ('customer','restaurant_owner','restaurant_manager','restaurant_staff',
              'rider','support_agent','ops_manager','admin')),
    PRIMARY KEY (user_id, role)
);
CREATE INDEX idx_user_roles_role ON user_roles(role);

CREATE TABLE credentials (
    user_id            uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    password_hash      text NOT NULL,          -- Argon2id PHC string; never plaintext
    must_change        boolean NOT NULL DEFAULT false,
    password_updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   bytea NOT NULL UNIQUE,          -- sha256 of the cookie token
    created_at   timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz,
    rotated_to   uuid REFERENCES sessions(id),
    last_seen_at timestamptz,
    ip           inet,
    user_agent   text
);
CREATE INDEX idx_sessions_user ON sessions(user_id) WHERE revoked_at IS NULL;

CREATE TABLE auth_events (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    event_type text NOT NULL CHECK (event_type IN
               ('register','login_success','login_failed','logout','logout_all',
                'password_reset_requested','password_reset_completed',
                'email_verified','session_rotated','account_deleted')),
    ip         inet,
    user_agent text,
    metadata   jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_auth_events_user ON auth_events(user_id, created_at DESC);

-- ===========================================================================
-- CUSTOMER PROFILE & ADDRESSES
-- ===========================================================================

CREATE TABLE customer_addresses (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id              uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    label                text NOT NULL DEFAULT 'other' CHECK (label IN ('home','work','other')),
    recipient_name       text NOT NULL,
    recipient_phone      varchar(20) NOT NULL,
    line1                text NOT NULL,
    line2                text,
    landmark             text,
    locality             text NOT NULL,
    city                 text NOT NULL,
    state                text NOT NULL,
    postal_code          varchar(10) NOT NULL,
    country              text NOT NULL DEFAULT 'IN',
    lat                  numeric(9,6),
    lng                  numeric(9,6),
    delivery_instructions text,
    is_default           boolean NOT NULL DEFAULT false,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now(),
    deleted_at           timestamptz
);
CREATE INDEX idx_addresses_user ON customer_addresses(user_id) WHERE deleted_at IS NULL;

CREATE TABLE customer_preferences (
    user_id             uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    is_veg_pref         boolean,
    preferred_cuisines  text[] NOT NULL DEFAULT '{}',
    language            text NOT NULL DEFAULT 'en',
    updated_at          timestamptz NOT NULL DEFAULT now()
);

-- ===========================================================================
-- SERVICE ZONES (demo: radius-based zones in the synthetic city of Bitepur)
-- ===========================================================================

CREATE TABLE service_zones (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name       text NOT NULL,
    city       text NOT NULL,
    center_lat numeric(9,6) NOT NULL,
    center_lng numeric(9,6) NOT NULL,
    radius_km  numeric(5,2) NOT NULL CHECK (radius_km > 0),
    is_active  boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now()
);

-- ===========================================================================
-- RESTAURANTS
-- ===========================================================================

CREATE TABLE restaurant_organisations (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    legal_name    text NOT NULL,
    display_name  text NOT NULL,
    owner_user_id uuid REFERENCES users(id),
    contact_email citext,
    contact_phone varchar(20),
    status        text NOT NULL DEFAULT 'draft'
                  CHECK (status IN ('draft','submitted','under_review','changes_requested',
                                    'approved','rejected','suspended')),
    gstin         varchar(15),
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE restaurant_applications (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id uuid NOT NULL REFERENCES restaurant_organisations(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'submitted'
                    CHECK (status IN ('submitted','under_review','changes_requested',
                                      'approved','rejected')),
    applicant_note  text,
    reviewer_id     uuid REFERENCES users(id),
    decision_note   text,
    decided_at      timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE application_events (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    application_id uuid NOT NULL REFERENCES restaurant_applications(id) ON DELETE CASCADE,
    from_status    text,
    to_status      text NOT NULL,
    actor_id       uuid REFERENCES users(id),
    note           text,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE outlets (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    organisation_id       uuid NOT NULL REFERENCES restaurant_organisations(id) ON DELETE CASCADE,
    name                  text NOT NULL,
    description           text,
    phone                 varchar(20) NOT NULL,
    address_line1         text NOT NULL,
    address_line2         text,
    locality              text NOT NULL,
    city                  text NOT NULL,
    state                 text NOT NULL,
    postal_code           varchar(10) NOT NULL,
    lat                   numeric(9,6) NOT NULL,
    lng                   numeric(9,6) NOT NULL,
    status                text NOT NULL DEFAULT 'active'
                          CHECK (status IN ('active','inactive','suspended')),
    is_pure_veg           boolean NOT NULL DEFAULT false,
    min_order_paise       bigint NOT NULL DEFAULT 0 CHECK (min_order_paise >= 0),
    packaging_fee_paise   bigint NOT NULL DEFAULT 0 CHECK (packaging_fee_paise >= 0),
    prep_minutes_min      int NOT NULL DEFAULT 15,
    prep_minutes_max      int NOT NULL DEFAULT 35,
    price_band            int NOT NULL DEFAULT 2 CHECK (price_band BETWEEN 1 AND 4),
    image_key             text,
    opens_all_day         boolean NOT NULL DEFAULT false,
    created_at            timestamptz NOT NULL DEFAULT now(),
    updated_at            timestamptz NOT NULL DEFAULT now(),
    deleted_at            timestamptz
);
CREATE INDEX idx_outlets_org ON outlets(organisation_id);
CREATE INDEX idx_outlets_city ON outlets(city, status) WHERE deleted_at IS NULL;

CREATE TABLE outlet_staff (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id  uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       text NOT NULL DEFAULT 'staff'
               CHECK (role IN ('owner','manager','staff')),
    granted_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (outlet_id, user_id)
);
CREATE INDEX idx_outlet_staff_user ON outlet_staff(user_id);

CREATE TABLE outlet_hours (
    outlet_id uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    weekday   smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),  -- 0=Sunday
    opens_at  time NOT NULL,
    closes_at time NOT NULL,
    is_closed boolean NOT NULL DEFAULT false,
    PRIMARY KEY (outlet_id, weekday)
);

CREATE TABLE outlet_holidays (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    holiday_date date NOT NULL,
    reason   text,
    UNIQUE (outlet_id, holiday_date)
);

CREATE TABLE outlet_service_areas (
    outlet_id uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    zone_id   uuid NOT NULL REFERENCES service_zones(id) ON DELETE CASCADE,
    PRIMARY KEY (outlet_id, zone_id)
);

CREATE TABLE cuisine_types (
    id   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE,
    slug text NOT NULL UNIQUE
);

CREATE TABLE outlet_cuisines (
    outlet_id  uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    cuisine_id uuid NOT NULL REFERENCES cuisine_types(id) ON DELETE CASCADE,
    PRIMARY KEY (outlet_id, cuisine_id)
);

CREATE TABLE outlet_collections (
    collection text NOT NULL,
    outlet_id  uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    PRIMARY KEY (collection, outlet_id)
);

-- ===========================================================================
-- MENU
-- ===========================================================================

CREATE TABLE menu_categories (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id     uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    name          text NOT NULL,
    display_order int NOT NULL DEFAULT 0,
    is_active     boolean NOT NULL DEFAULT true,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_menu_categories_outlet ON menu_categories(outlet_id, display_order);

CREATE TABLE menu_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id      uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    category_id    uuid REFERENCES menu_categories(id) ON DELETE SET NULL,
    name           text NOT NULL,
    description    text,
    price_paise    bigint NOT NULL CHECK (price_paise >= 0),
    tax_category   text NOT NULL DEFAULT 'gst_5' CHECK (tax_category IN ('gst_5','gst_12','exempt')),
    is_veg         boolean NOT NULL DEFAULT true,
    dietary_tags   text[] NOT NULL DEFAULT '{}',
    allergen_notes text,
    serving_info   text,
    image_key      text,
    is_featured    boolean NOT NULL DEFAULT false,
    is_bestseller  boolean NOT NULL DEFAULT false,
    is_available   boolean NOT NULL DEFAULT true,
    display_order  int NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    deleted_at     timestamptz
);
CREATE INDEX idx_menu_items_outlet ON menu_items(outlet_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_menu_items_category ON menu_items(category_id);

CREATE TABLE modifier_groups (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id     uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    name          text NOT NULL,
    is_required   boolean NOT NULL DEFAULT false,
    min_select    int NOT NULL DEFAULT 0,
    max_select    int NOT NULL DEFAULT 1,
    display_order int NOT NULL DEFAULT 0
);
CREATE INDEX idx_modifier_groups_outlet ON modifier_groups(outlet_id);

CREATE TABLE modifier_options (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id          uuid NOT NULL REFERENCES modifier_groups(id) ON DELETE CASCADE,
    name              text NOT NULL,
    price_delta_paise bigint NOT NULL DEFAULT 0 CHECK (price_delta_paise >= 0),
    is_default        boolean NOT NULL DEFAULT false,
    is_available      boolean NOT NULL DEFAULT true
);
CREATE INDEX idx_modifier_options_group ON modifier_options(group_id);

CREATE TABLE item_modifier_groups (
    item_id       uuid NOT NULL REFERENCES menu_items(id) ON DELETE CASCADE,
    group_id      uuid NOT NULL REFERENCES modifier_groups(id) ON DELETE CASCADE,
    display_order int NOT NULL DEFAULT 0,
    PRIMARY KEY (item_id, group_id)
);

-- ===========================================================================
-- CART
-- ===========================================================================

CREATE TABLE carts (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    outlet_id  uuid NOT NULL REFERENCES outlets(id),
    status     text NOT NULL DEFAULT 'active' CHECK (status IN ('active','converted','abandoned')),
    note       text CHECK (note IS NULL OR length(note) <= 500),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_carts_outlet ON carts(outlet_id);

CREATE TABLE cart_items (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id              uuid NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
    menu_item_id         uuid NOT NULL REFERENCES menu_items(id),
    quantity             int NOT NULL CHECK (quantity BETWEEN 1 AND 20),
    special_instructions text CHECK (special_instructions IS NULL OR length(special_instructions) <= 240),
    -- snapshots captured at add-time; revalidated against live menu at quote/checkout
    name_snapshot        text NOT NULL,
    price_snapshot_paise bigint NOT NULL,
    is_veg_snapshot      boolean NOT NULL,
    created_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_cart_items_cart ON cart_items(cart_id);

CREATE TABLE cart_item_modifiers (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_item_id        uuid NOT NULL REFERENCES cart_items(id) ON DELETE CASCADE,
    group_id            uuid NOT NULL REFERENCES modifier_groups(id),
    option_id           uuid NOT NULL REFERENCES modifier_options(id),
    group_name_snapshot  text NOT NULL,
    option_name_snapshot text NOT NULL,
    price_delta_snapshot bigint NOT NULL DEFAULT 0
);
CREATE INDEX idx_cart_item_modifiers_item ON cart_item_modifiers(cart_item_id);

-- ===========================================================================
-- PRICING & QUOTES
-- ===========================================================================

CREATE TABLE pricing_rules (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version        int NOT NULL,
    name           text NOT NULL,
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    is_active      boolean NOT NULL DEFAULT true,
    config         jsonb NOT NULL,   -- delivery fee tables, platform fee, surge, small-order
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (version, name)
);

CREATE TABLE commission_rules (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id      uuid REFERENCES outlets(id),  -- NULL = global default
    commission_bp  int NOT NULL CHECK (commission_bp BETWEEN 0 AND 10000), -- basis points
    effective_from timestamptz NOT NULL,
    effective_to   timestamptz,
    created_by     uuid REFERENCES users(id),
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE pricing_quotes (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    outlet_id         uuid NOT NULL REFERENCES outlets(id),
    cart_fingerprint  text NOT NULL,           -- hash of cart contents + modifiers
    address_id        uuid REFERENCES customer_addresses(id),
    coupon_id         uuid,
    pricing_rule_id   uuid REFERENCES pricing_rules(id),
    breakdown         jsonb NOT NULL,          -- full line items
    grand_total_paise bigint NOT NULL,
    version           int NOT NULL,
    expires_at        timestamptz NOT NULL,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_quotes_user ON pricing_quotes(user_id, expires_at DESC);

-- ===========================================================================
-- ORDERS
-- ===========================================================================

CREATE TABLE orders (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reference              text NOT NULL UNIQUE,
    customer_id            uuid NOT NULL REFERENCES users(id),
    outlet_id              uuid NOT NULL REFERENCES outlets(id),
    organisation_id        uuid NOT NULL REFERENCES restaurant_organisations(id),
    status                 text NOT NULL DEFAULT 'pending_payment'
                           CHECK (status IN ('pending_payment','placed','restaurant_confirmed',
                                 'preparing','ready_for_pickup','rider_assigned','picked_up',
                                 'on_the_way','delivered','rejected','cancel_requested',
                                 'cancelled','delivery_failed')),
    mode                   text NOT NULL DEFAULT 'delivery' CHECK (mode IN ('delivery')),
    placed_at              timestamptz,
    confirmed_at           timestamptz,
    preparing_at           timestamptz,
    ready_at               timestamptz,
    picked_at              timestamptz,
    delivered_at           timestamptz,
    closed_at              timestamptz,
    eta_min_minutes        int,
    eta_max_minutes        int,
    prep_estimate_minutes  int,
    instructions           text CHECK (instructions IS NULL OR length(instructions) <= 500),
    -- authoritative price snapshot (never recomputed later)
    currency               text NOT NULL DEFAULT 'INR',
    items_subtotal_paise   bigint NOT NULL DEFAULT 0,
    item_discounts_paise   bigint NOT NULL DEFAULT 0,
    coupon_discount_paise  bigint NOT NULL DEFAULT 0,
    packaging_fee_paise    bigint NOT NULL DEFAULT 0,
    delivery_fee_paise     bigint NOT NULL DEFAULT 0,
    platform_fee_paise     bigint NOT NULL DEFAULT 0,
    small_order_fee_paise  bigint NOT NULL DEFAULT 0,
    surge_fee_paise        bigint NOT NULL DEFAULT 0,
    tip_paise              bigint NOT NULL DEFAULT 0,
    tax_paise              bigint NOT NULL DEFAULT 0,
    grand_total_paise      bigint NOT NULL DEFAULT 0,
    pricing_snapshot       jsonb NOT NULL DEFAULT '{}',
    pricing_rule_id        uuid REFERENCES pricing_rules(id),
    quote_id               uuid REFERENCES pricing_quotes(id),
    coupon_id              uuid, -- FK added after coupons table exists
    rejection_reason       text,
    cancel_reason          text,
    cancelled_by_actor     text,
    rider_queue_reason     text,
    created_at             timestamptz NOT NULL DEFAULT now(),
    updated_at             timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_orders_customer ON orders(customer_id, created_at DESC);
CREATE INDEX idx_orders_outlet ON orders(outlet_id, created_at DESC);
CREATE INDEX idx_orders_status ON orders(status) WHERE closed_at IS NULL;

CREATE TABLE order_items (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    menu_item_id      uuid REFERENCES menu_items(id),   -- nullable: item may be deleted later
    name_snapshot     text NOT NULL,
    is_veg_snapshot   boolean NOT NULL,
    quantity          int NOT NULL CHECK (quantity > 0),
    unit_price_paise  bigint NOT NULL,
    item_total_paise  bigint NOT NULL,
    special_instructions text,
    modifiers_total_paise bigint NOT NULL DEFAULT 0
);
CREATE INDEX idx_order_items_order ON order_items(order_id);

CREATE TABLE order_item_modifiers (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_item_id        uuid NOT NULL REFERENCES order_items(id) ON DELETE CASCADE,
    group_name_snapshot  text NOT NULL,
    option_name_snapshot text NOT NULL,
    price_delta_paise    bigint NOT NULL DEFAULT 0
);

CREATE TABLE order_addresses (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        uuid NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
    recipient_name  text NOT NULL,
    recipient_phone varchar(20) NOT NULL,
    line1           text NOT NULL,
    line2           text,
    landmark        text,
    locality        text NOT NULL,
    city            text NOT NULL,
    state           text NOT NULL,
    postal_code     varchar(10) NOT NULL,
    lat             numeric(9,6),
    lng             numeric(9,6),
    instructions    text,
    zone_id         uuid REFERENCES service_zones(id)
);

CREATE TABLE order_events (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_id       uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    event_type     text NOT NULL,
    from_state     text,
    to_state       text,
    actor_type     text NOT NULL CHECK (actor_type IN ('customer','restaurant','rider','admin','support','system','dispatch')),
    actor_id       uuid,
    reason         text,
    metadata       jsonb NOT NULL DEFAULT '{}',
    correlation_id uuid,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_order_events_order ON order_events(order_id, id);

-- ===========================================================================
-- PAYMENTS & REFUNDS
-- ===========================================================================

CREATE TABLE payments (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id          uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    provider          text NOT NULL,
    method            text NOT NULL CHECK (method IN ('cod','mock_upi','mock_card','mock_netbanking')),
    status            text NOT NULL CHECK (status IN
                      ('pending','requires_action','authorised','captured','failed','cancelled',
                       'partially_refunded','refunded','cod_due','cod_collected')),
    amount_paise      bigint NOT NULL CHECK (amount_paise >= 0),
    currency          text NOT NULL DEFAULT 'INR',
    provider_reference text,
    captured_at       timestamptz,
    failure_reason    text,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_payments_order ON payments(order_id);
CREATE INDEX idx_payments_provider_reference ON payments(provider_reference);

CREATE TABLE payment_attempts (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id         uuid NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
    attempt_no         int NOT NULL,
    status             text NOT NULL,
    provider_reference text,
    detail             jsonb NOT NULL DEFAULT '{}',
    created_at         timestamptz NOT NULL DEFAULT now(),
    UNIQUE (payment_id, attempt_no)
);

CREATE TABLE payment_webhook_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    provider        text NOT NULL,
    event_id        text NOT NULL,
    payload         jsonb NOT NULL,
    signature_valid boolean NOT NULL,
    processed_at    timestamptz,
    processing_error text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, event_id)
);

CREATE TABLE refunds (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id         uuid NOT NULL REFERENCES payments(id),
    order_id           uuid NOT NULL REFERENCES orders(id),
    amount_paise       bigint NOT NULL CHECK (amount_paise > 0),
    status             text NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending','processed','failed','cancelled')),
    reason             text NOT NULL,
    provider_reference text,
    initiated_by       text NOT NULL CHECK (initiated_by IN ('customer','admin','support','system')),
    created_at         timestamptz NOT NULL DEFAULT now(),
    completed_at       timestamptz
);
CREATE INDEX idx_refunds_order ON refunds(order_id);

-- ===========================================================================
-- DELIVERY PARTNERS
-- ===========================================================================

CREATE TABLE delivery_partners (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         uuid NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    status          text NOT NULL DEFAULT 'application_pending'
                    CHECK (status IN ('application_pending','approved','rejected','suspended')),
    availability    text NOT NULL DEFAULT 'offline'
                    CHECK (availability IN ('online','offline')),
    last_online_at  timestamptz,
    vehicle_type    text NOT NULL DEFAULT 'bike'
                    CHECK (vehicle_type IN ('bicycle','bike','scooter','ev')),
    vehicle_number  text,
    base_zone_id    uuid REFERENCES service_zones(id),
    current_lat     numeric(9,6),
    current_lng     numeric(9,6),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE rider_applications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status      text NOT NULL DEFAULT 'submitted'
                CHECK (status IN ('submitted','under_review','approved','rejected')),
    note        text,
    reviewer_id uuid REFERENCES users(id),
    decided_at  timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE delivery_assignments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id       uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    rider_id       uuid REFERENCES delivery_partners(id),
    status         text NOT NULL DEFAULT 'assigned'
                   CHECK (status IN ('assigned','accepted','arriving_at_restaurant',
                          'arrived_at_restaurant','picked_up','on_the_way','delivered',
                          'delivery_failed','cancelled','expired','rejected')),
    offered_at     timestamptz NOT NULL DEFAULT now(),
    accepted_at    timestamptz,
    expires_at     timestamptz NOT NULL,
    pickup_otp_hash text,                       -- sha256 of short numeric OTP
    distance_km    numeric(6,2),
    assigned_by    text NOT NULL DEFAULT 'dispatch'
                   CHECK (assigned_by IN ('dispatch','admin')),
    payout_base_paise   bigint NOT NULL DEFAULT 0,
    payout_incentive_paise bigint NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_assignments_order ON delivery_assignments(order_id);
CREATE INDEX idx_assignments_rider ON delivery_assignments(rider_id, status);
CREATE INDEX idx_assignments_open ON delivery_assignments(status) WHERE status IN ('assigned');

CREATE TABLE delivery_events (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    assignment_id uuid NOT NULL REFERENCES delivery_assignments(id) ON DELETE CASCADE,
    event_type    text NOT NULL,
    actor_type    text NOT NULL,
    actor_id      uuid,
    metadata      jsonb NOT NULL DEFAULT '{}',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_delivery_events_assignment ON delivery_events(assignment_id, id);

-- Demo rider coordinates with retention: rows older than expires_at are pruned.
CREATE TABLE delivery_locations (
    id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    assignment_id uuid NOT NULL REFERENCES delivery_assignments(id) ON DELETE CASCADE,
    lat           numeric(9,6) NOT NULL,
    lng           numeric(9,6) NOT NULL,
    accuracy_m    int,
    source        text NOT NULL DEFAULT 'demo_sim' CHECK (source IN ('demo_sim','device')),
    recorded_at   timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX idx_delivery_locations_assignment ON delivery_locations(assignment_id, recorded_at);

CREATE TABLE rider_earnings (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rider_id      uuid NOT NULL REFERENCES delivery_partners(id),
    assignment_id uuid NOT NULL UNIQUE REFERENCES delivery_assignments(id),
    base_pay_paise     bigint NOT NULL DEFAULT 0,
    incentive_paise    bigint NOT NULL DEFAULT 0,
    tip_paise          bigint NOT NULL DEFAULT 0,
    adjustments_paise  bigint NOT NULL DEFAULT 0,
    total_paise        bigint NOT NULL DEFAULT 0,
    status        text NOT NULL DEFAULT 'payable' CHECK (status IN ('payable','settled')),
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_rider_earnings_rider ON rider_earnings(rider_id, created_at DESC);

-- ===========================================================================
-- PROMOTIONS
-- ===========================================================================

CREATE TABLE coupons (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    code                  citext NOT NULL UNIQUE,
    title                 text NOT NULL,
    description           text,
    discount_type         text NOT NULL CHECK (discount_type IN ('percent','fixed')),
    percent_bp            int CHECK (percent_bp BETWEEN 1 AND 10000),
    fixed_paise           bigint CHECK (fixed_paise > 0),
    max_cap_paise         bigint,
    min_order_paise       bigint NOT NULL DEFAULT 0,
    starts_at             timestamptz NOT NULL,
    ends_at               timestamptz,
    is_active             boolean NOT NULL DEFAULT true,
    usage_limit_per_user  int NOT NULL DEFAULT 1,
    total_redemption_limit int,
    redeemed_count        int NOT NULL DEFAULT 0,
    funding_party         text NOT NULL DEFAULT 'platform' CHECK (funding_party IN ('platform','restaurant')),
    eligible_outlet_id    uuid REFERENCES outlets(id),
    new_users_only        boolean NOT NULL DEFAULT false,
    created_by            uuid REFERENCES users(id),
    created_at            timestamptz NOT NULL DEFAULT now(),
    CHECK (
        (discount_type = 'percent' AND percent_bp IS NOT NULL) OR
        (discount_type = 'fixed' AND fixed_paise IS NOT NULL)
    )
);

CREATE TABLE coupon_redemptions (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    coupon_id      uuid NOT NULL REFERENCES coupons(id),
    user_id        uuid NOT NULL REFERENCES users(id),
    order_id       uuid NOT NULL UNIQUE REFERENCES orders(id),
    discount_paise bigint NOT NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_redemptions_coupon_user ON coupon_redemptions(coupon_id, user_id);

-- ===========================================================================
-- REVIEWS, FAVOURITES
-- ===========================================================================

CREATE TABLE reviews (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id        uuid NOT NULL UNIQUE REFERENCES orders(id),
    customer_id     uuid NOT NULL REFERENCES users(id),
    outlet_id       uuid NOT NULL REFERENCES outlets(id),
    rating          smallint NOT NULL CHECK (rating BETWEEN 1 AND 5),
    delivery_rating smallint CHECK (delivery_rating BETWEEN 1 AND 5),
    comment         text CHECK (comment IS NULL OR length(comment) <= 2000),
    status          text NOT NULL DEFAULT 'published' CHECK (status IN ('published','hidden')),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_reviews_outlet ON reviews(outlet_id, status);
CREATE INDEX idx_reviews_customer ON reviews(customer_id);

CREATE TABLE review_reports (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    review_id   uuid NOT NULL REFERENCES reviews(id) ON DELETE CASCADE,
    reporter_id uuid REFERENCES users(id),
    reason      text NOT NULL,
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open','resolved')),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE favourites (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    outlet_id  uuid NOT NULL REFERENCES outlets(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, outlet_id)
);

CREATE TABLE search_history (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    query      text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_search_history_user ON search_history(user_id, created_at DESC);

-- ===========================================================================
-- SUPPORT
-- ===========================================================================

CREATE TABLE support_tickets (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    reference    text NOT NULL UNIQUE,
    customer_id  uuid REFERENCES users(id),
    order_id     uuid REFERENCES orders(id),
    category     text NOT NULL CHECK (category IN
                 ('missing_item','wrong_item','quality','late_delivery','payment',
                  'cancellation_refund','conduct','account','other')),
    subject      text NOT NULL,
    status       text NOT NULL DEFAULT 'open'
                 CHECK (status IN ('open','assigned','waiting_for_customer','resolved','closed')),
    priority     text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
    assigned_to  uuid REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    resolved_at  timestamptz
);
CREATE INDEX idx_tickets_customer ON support_tickets(customer_id, created_at DESC);
CREATE INDEX idx_tickets_status ON support_tickets(status);

CREATE TABLE support_messages (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id         uuid NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    author_type       text NOT NULL CHECK (author_type IN ('customer','agent','system')),
    author_id         uuid,
    body              text NOT NULL,
    is_internal_note  boolean NOT NULL DEFAULT false,
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_support_messages_ticket ON support_messages(ticket_id, created_at);

CREATE TABLE support_attachments (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    ticket_id   uuid NOT NULL REFERENCES support_tickets(id) ON DELETE CASCADE,
    message_id  uuid REFERENCES support_messages(id) ON DELETE SET NULL,
    storage_key text NOT NULL,
    filename    text NOT NULL,
    mime        text NOT NULL,
    size_bytes  bigint NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- ===========================================================================
-- NOTIFICATIONS & OUTBOX
-- ===========================================================================

CREATE TABLE notifications (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category   text NOT NULL,
    title      text NOT NULL,
    body       text NOT NULL,
    metadata   jsonb NOT NULL DEFAULT '{}',
    read_at    timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications(user_id, created_at DESC);

CREATE TABLE notification_preferences (
    user_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category   text NOT NULL,
    inapp      boolean NOT NULL DEFAULT true,
    email      boolean NOT NULL DEFAULT true,
    sms        boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, category)
);

CREATE TABLE notification_deliveries (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    notification_id uuid NOT NULL REFERENCES notifications(id) ON DELETE CASCADE,
    channel         text NOT NULL CHECK (channel IN ('inapp','email','sms')),
    status          text NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued','sent','failed','skipped')),
    provider_ref    text,
    detail          text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    sent_at         timestamptz,
    UNIQUE (notification_id, channel)
);

CREATE TABLE outbox_events (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type  text NOT NULL,
    aggregate_id    uuid NOT NULL,
    event_type      text NOT NULL,
    payload         jsonb NOT NULL DEFAULT '{}',
    dedup_key       text UNIQUE,
    status          text NOT NULL DEFAULT 'pending'
                    CHECK (status IN ('pending','processing','done','dead')),
    attempts        int NOT NULL DEFAULT 0,
    max_attempts    int NOT NULL DEFAULT 5,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error      text,
    created_at      timestamptz NOT NULL DEFAULT now(),
    processed_at    timestamptz
);
CREATE INDEX idx_outbox_pending ON outbox_events(status, next_attempt_at) WHERE status IN ('pending','processing');

-- ===========================================================================
-- PLATFORM: audit, idempotency, flags, settings, settlements
-- ===========================================================================

CREATE TABLE audit_logs (
    id             bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_type     text NOT NULL CHECK (actor_type IN ('user','system')),
    actor_id       uuid,
    action         text NOT NULL,
    target_type    text NOT NULL,
    target_id      text,
    before_state   jsonb,
    after_state    jsonb,
    reason         text,
    correlation_id uuid,
    ip             inet,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_target ON audit_logs(target_type, target_id, created_at DESC);
CREATE INDEX idx_audit_actor ON audit_logs(actor_id, created_at DESC);

CREATE TABLE idempotency_records (
    key             text PRIMARY KEY,
    endpoint        text NOT NULL,
    user_id         uuid,
    request_hash    text NOT NULL,
    response_status int,
    response_body   jsonb,
    created_at      timestamptz NOT NULL DEFAULT now(),
    expires_at      timestamptz NOT NULL
);
CREATE INDEX idx_idempotency_expiry ON idempotency_records(expires_at);

CREATE TABLE feature_flags (
    key         text PRIMARY KEY,
    enabled     boolean NOT NULL DEFAULT false,
    description text NOT NULL DEFAULT '',
    updated_by  uuid REFERENCES users(id),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE platform_settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_by uuid REFERENCES users(id),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE restaurant_settlements (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    outlet_id      uuid NOT NULL REFERENCES outlets(id),
    period_start   date NOT NULL,
    period_end     date NOT NULL,
    gov_paise      bigint NOT NULL DEFAULT 0,
    commission_paise bigint NOT NULL DEFAULT 0,
    restaurant_funded_discounts_paise bigint NOT NULL DEFAULT 0,
    adjustments_paise bigint NOT NULL DEFAULT 0,
    payable_paise  bigint NOT NULL DEFAULT 0,
    status         text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','finalised','paid','disputed')),
    finalised_at   timestamptz,
    paid_at        timestamptz,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (outlet_id, period_start, period_end)
);

CREATE TABLE rider_settlements (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    rider_id      uuid NOT NULL REFERENCES delivery_partners(id),
    period_start  date NOT NULL,
    period_end    date NOT NULL,
    payable_paise bigint NOT NULL DEFAULT 0,
    status        text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','finalised','paid')),
    finalised_at  timestamptz,
    paid_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (rider_id, period_start, period_end)
);

-- Forward foreign keys that reference tables created later in this file.
ALTER TABLE orders
    ADD CONSTRAINT fk_orders_coupon FOREIGN KEY (coupon_id) REFERENCES coupons(id);

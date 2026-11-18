-- Quotes must carry their payment method and ETA range so placement never
-- depends on client-supplied values (ADR: server authority over checkout).
ALTER TABLE pricing_quotes
    ADD COLUMN IF NOT EXISTS payment_method text,
    ADD COLUMN IF NOT EXISTS eta_min_minutes int,
    ADD COLUMN IF NOT EXISTS eta_max_minutes int;

ALTER TABLE pricing_quotes
    ADD CONSTRAINT fk_quotes_payment_method CHECK (payment_method IN ('cod','mock_upi','mock_card','mock_netbanking'));

-- Haversine distance in km (straight-line estimate only; NOT road distance).
CREATE OR REPLACE FUNCTION geo_distance_km(lat1 float8, lng1 float8, lat2 float8, lng2 float8)
RETURNS float8 AS $$
    SELECT 6371.0 * 2 * asin(sqrt(
        least(1.0,
            power(sin(radians(lat2 - lat1) / 2), 2) +
            cos(radians(lat1)) * cos(radians(lat2)) *
            power(sin(radians(lng2 - lng1) / 2), 2)
        )
    ));
$$ LANGUAGE sql IMMUTABLE;

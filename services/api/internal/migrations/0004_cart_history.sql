-- Carts keep history: exactly one ACTIVE cart per user, but converted
-- carts remain for audit. The blanket UNIQUE(user_id) blocked re-ordering.
ALTER TABLE carts DROP CONSTRAINT IF EXISTS carts_user_id_key;
CREATE UNIQUE INDEX IF NOT EXISTS idx_carts_user_active ON carts(user_id) WHERE status = 'active';

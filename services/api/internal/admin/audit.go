package admin

import (
	"context"
	"encoding/json"
	"time"

	"biteorbit/services/api/internal/db"
)

// audit writes the mandatory admin audit row (actor, target, before/after,
// reason) inside the current transaction when one is provided.
func audit(ctx context.Context, q db.Querier, actorID, action, targetType, targetID string,
	before, after map[string]any, reason string,
) error {
	beforeRaw := "{}"
	if before != nil {
		if raw, err := json.Marshal(before); err == nil {
			beforeRaw = string(raw)
		}
	}
	afterRaw := "{}"
	if after != nil {
		if raw, err := json.Marshal(after); err == nil {
			afterRaw = string(raw)
		}
	}
	_, err := q.Exec(ctx, `
		INSERT INTO audit_logs (actor_type, actor_id, action, target_type, target_id, before_state, after_state, reason)
		VALUES ('user', $1, $2, $3, $4, $5::jsonb, $6::jsonb, NULLIF($7,''))`,
		actorID, action, targetType, targetID, beforeRaw, afterRaw, reason)
	return err
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	s := ""
	for v > 0 {
		s = string(rune('0'+v%10)) + s
		v /= 10
	}
	return s
}

var _ = time.Now

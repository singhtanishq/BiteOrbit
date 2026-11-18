// Package outbox implements the transactional outbox pattern: events that
// must trigger notifications/jobs are committed in the same transaction as
// the state change, then dispatched asynchronously with retries and a
// dead-letter state. Duplicate deliveries are prevented via dedup keys, and
// handlers must be idempotent.
package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"biteorbit/services/api/internal/db"
)

// Enqueue inserts an event in the CURRENT TRANSACTION (pass the tx as q) so
// that committing the business change also commits its follow-up event.
func Enqueue(ctx context.Context, q db.Querier, aggregateType, aggregateID, eventType string, payload map[string]any, dedupKey *string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal outbox payload: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO outbox_events (aggregate_type, aggregate_id, event_type, payload, dedup_key)
		VALUES ($1, $2, $3, $4::jsonb, $5)
		ON CONFLICT (dedup_key) DO NOTHING`,
		aggregateType, aggregateID, eventType, string(raw), dedupKey)
	return err
}

// Handler processes one event. Handlers MUST be idempotent: retries are
// guaranteed to happen after failures and possible duplicate dispatches.
type Handler interface {
	Handle(ctx context.Context, evt Event) error
	Name() string
}

// Event is the dispatchable shape.
type Event struct {
	ID            string          `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   string          `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
}

// Dispatcher polls the outbox and fans events out to registered handlers.
type Dispatcher struct {
	pool     *pgxpool.Pool
	log      *slog.Logger
	handlers map[string][]Handler // event_type -> handlers
	interval time.Duration
	stop     chan struct{}
}

func NewDispatcher(pool *pgxpool.Pool, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		pool:     pool,
		log:      log,
		handlers: map[string][]Handler{},
		interval: 2 * time.Second,
		stop:     make(chan struct{}),
	}
}

// Register subscribes a handler to an event type.
func (d *Dispatcher) Register(eventType string, h Handler) {
	d.handlers[eventType] = append(d.handlers[eventType], h)
}

// Run processes the queue until Stop. Retry policy: exponential backoff
// (2^attempts * 5s) up to max_attempts, then status='dead' for inspection.
func (d *Dispatcher) Run() {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	for {
		select {
		case <-d.stop:
			return
		case <-ticker.C:
			d.processBatch(context.Background())
		}
	}
}

func (d *Dispatcher) Stop() { close(d.stop) }

func (d *Dispatcher) processBatch(ctx context.Context) {
	bctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	rows, err := d.pool.Query(bctx, `
		UPDATE outbox_events
		SET status = 'processing', attempts = attempts + 1
		WHERE id IN (
			SELECT id FROM outbox_events
			WHERE status = 'pending' AND next_attempt_at <= now()
			ORDER BY created_at
			LIMIT 20
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, aggregate_type, aggregate_id, event_type, payload, attempts, max_attempts`)
	if err != nil {
		d.log.Error("outbox claim failed", slog.Any("err", err))
		return
	}
	type claimed struct {
		id            string
		aggregateType string
		aggregateID   string
		eventType     string
		payload       []byte
		attempts      int
		maxAttempts   int
	}
	var batch []claimed
	for rows.Next() {
		var c claimed
		if err := rows.Scan(&c.id, &c.aggregateType, &c.aggregateID, &c.eventType, &c.payload, &c.attempts, &c.maxAttempts); err == nil {
			batch = append(batch, c)
		}
	}
	rows.Close()

	for _, c := range batch {
		evt := Event{ID: c.id, AggregateType: c.aggregateType, AggregateID: c.aggregateID,
			EventType: c.eventType, Payload: c.payload}
		handlers := d.handlers[c.eventType]
		var firstErr error
		handledAny := false
		for _, h := range handlers {
			hctx, hcancel := context.WithTimeout(bctx, 10*time.Second)
			err := h.Handle(hctx, evt)
			hcancel()
			if err != nil {
				firstErr = err
			} else {
				handledAny = true
			}
		}
		switch {
		case len(handlers) == 0:
			d.markDone(bctx, c.id) // no subscriber: treat as delivered
		case firstErr == nil:
			d.markDone(bctx, c.id)
		case handledAny:
			// Partial success: keep pending for retry of the failing handlers.
			d.markRetry(bctx, c.id, c.attempts, c.maxAttempts, firstErr)
		default:
			d.markRetry(bctx, c.id, c.attempts, c.maxAttempts, firstErr)
		}
	}
}

func (d *Dispatcher) markDone(ctx context.Context, id string) {
	_, err := d.pool.Exec(ctx,
		`UPDATE outbox_events SET status='done', processed_at=now(), last_error=NULL WHERE id=$1`, id)
	if err != nil {
		d.log.Error("outbox mark done failed", slog.String("id", id), slog.Any("err", err))
	}
}

func (d *Dispatcher) markRetry(ctx context.Context, id string, attempts, maxAttempts int, cause error) {
	if attempts >= maxAttempts {
		_, err := d.pool.Exec(ctx,
			`UPDATE outbox_events SET status='dead', last_error=$2 WHERE id=$1`, id, cause.Error())
		if err == nil {
			d.log.Error("outbox event dead-lettered", slog.String("id", id), slog.Any("err", cause))
		}
		return
	}
	backoff := time.Duration(1<<uint(attempts)) * 5 * time.Second
	if backoff > 10*time.Minute {
		backoff = 10 * time.Minute
	}
	_, err := d.pool.Exec(ctx,
		`UPDATE outbox_events SET status='pending', next_attempt_at=now()+$2, last_error=$3 WHERE id=$1`,
		id, backoff, cause.Error())
	if err != nil {
		d.log.Error("outbox mark retry failed", slog.String("id", id), slog.Any("err", err))
	}
}

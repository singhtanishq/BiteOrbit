# Order lifecycle state machine

The state machine lives in the Go API (`internal/orders/state_machine.go`) and is enforced
inside a transaction for every transition. Every transition appends an immutable
`order_events` row (actor, from/to state, reason, metadata) and emits outbox events.

```
                       ┌──────────────┐
      COD / paid ───►  │    placed    │
                       └──┬───────┬───┘
              accept     │       │ reject (restaurant, timeout)
                        ▼       ▼
              ┌────────────┐   ┌──────────┐
              │ restaurant │   │ rejected │──► refund (if paid)
              │ _confirmed │   └──────────┘
              └──────┬─────┘
                     ▼
              ┌────────────┐   customer cancel (policy window)
              │ preparing  │──────────────► ┌────────────────┐
              └──────┬─────┘                │ cancel_requested│──► cancelled
                     ▼                      └────────────────┘
              ┌───────────────┐
              │ ready_for_    │
              │ pickup        │
              └──────┬────────┘
                     ▼ dispatch (assignment accepted)
              ┌──────────────┐
              │ rider_assigned│
              └──────┬───────┘
                     ▼ picked_up
              ┌──────────────┐   failure at door
              │ on_the_way   │──────────► delivery_failed
              └──────┬───────┘            (refund policy)
                     ▼
              ┌──────────┐
              │ delivered│  (terminal; COD marked collected)
              └──────────┘
```

Rules enforced server-side (see tests in `internal/orders/*_test.go`):

1. Only listed transitions are legal; anything else returns `409 invalid_state_transition`.
2. Actors: customer (cancel within policy window, from `placed|restaurant_confirmed|preparing`
   before rider_assigned), restaurant (confirm/reject/preparing/ready), dispatch service
   (rider_assigned), rider (picked_up/on_the_way/delivered/delivery_failed), admin
   (audited intervention incl. cancellation).
3. Terminal states (`delivered`, `rejected`, `cancelled`, `delivery_failed`) cannot transition;
   corrections happen only via audited admin workflows that create refunds/new records.
4. Payment-aware guards: online orders sit in `pending_payment` until captured; cancelled paid
   orders produce refund records; COD uses its own due/collected lifecycle.
5. Concurrency: transitions use atomic conditional UPDATE … WHERE state = expected, so double
   accept / double deliver races lose safely and report the actual current state.

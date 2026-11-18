# BiteOrbit architecture overview

## System context

BiteOrbit is a multi-sided marketplace. Four browser experiences talk to a single
versioned Go REST API, which owns all business-critical state. PostgreSQL is the
source of truth; Redis provides rate limiting and pub/sub fan-out with an
in-process fallback; Mailpit captures local email; provider adapters isolate
payments, SMS, email, maps and object storage.

```
                    ┌──────────────────────────────────────────────┐
                    │                Browsers (PWA-ready)          │
                    │  customer :3000   restaurant :3001           │
                    │  courier  :3002   admin     :3003            │
                    └──────────────┬───────────────────────────────┘
                                   │ same-origin /api/v1/* proxy
                                   ▼
                    ┌──────────────────────────────┐
                    │        Go API :8080          │
                    │  HTTP + SSE, RBAC, pricing   │
                    │  orders state machine, outbox│
                    └───┬─────────┬─────────┬──────┘
                        │         │         │
             ┌──────────▼──┐ ┌────▼────┐ ┌──▼──────────────┐
             │ PostgreSQL  │ │  Redis  │ │ Provider        │
             │ (migrations │ │ ratelmt │ │ adapters:       │
             │  + data)    │ │ pubsub  │ │ payments(mock)  │
             └─────────────┘ └─────────┘ │ sms/email(fake, │
                                         │ Mailpit) maps   │
             ┌─────────────┐             │ (demo sim)      │
             │ Outbox      │────────────►│ storage(filesys)│
             │ dispatcher  │  jobs/notify └────────────────┘
             └─────────────┘
```

## Repositories of truth

| Concern                     | Owner                                          |
| --------------------------- | ---------------------------------------------- |
| Money, taxes, fees, coupons | Go API pricing engine (integer paise)          |
| Order lifecycle transitions | Go API state machine (append-only events)      |
| Payment states & refunds    | Go API payment service + webhook verification  |
| Rider assignment            | Go API dispatch service (transactional claim)  |
| Roles & record access       | Go API RBAC middleware + record-scoped queries |

Frontends never compute an authoritative amount and never set order state.

## Experiences

- **Customer** (`apps/customer-web`): discovery, menus, cart, checkout, tracking, reviews, history.
- **Restaurant** (`apps/restaurant-web`): order queue, fulfilment, menu CRUD, settings, analytics.
- **Courier** (`apps/courier-web`): availability, offers, pickup/drop workflow, earnings.
- **Admin** (`apps/admin-web`): oversight, applications, users, orders, promotions, support, audit.

## Key flows

1. **Order placement**: cart → server quote (prices recomputed) → idempotent placement with
   payment intent → order event stream via outbox → SSE/polling updates.
2. **Fulfilment**: restaurant accept/reject → prepare → ready → dispatch claims a rider
   (atomic) → pickup → delivery → (COD collected if applicable).
3. **Failure paths**: rejection, no-rider queue, payment failure, cancellation windows,
   refunds (simulated locally, provider-confirmed in production).

Domain glossary lives in [glossary.md](glossary.md); the order state machine is documented in
[state-machine.md](state-machine.md) and enforced in `services/api/internal/orders`.

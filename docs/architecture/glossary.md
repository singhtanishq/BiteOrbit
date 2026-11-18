# BiteOrbit domain glossary

| Term                | Meaning                                                                             |
| ------------------- | ----------------------------------------------------------------------------------- |
| **Organisation**    | A restaurant business entity (legal owner of one or more outlets).                  |
| **Outlet**          | A physical/operational restaurant location that accepts orders.                     |
| **Service zone**    | A configurable delivery area (demo: seeded localities of Bitepur).                  |
| **Menu item**       | A sellable dish with price, tax category, dietary flags, availability.              |
| **Modifier group**  | A set of options attached to an item (required or optional, min/max).               |
| **Cart**            | Single-restaurant persisted selection of configured items for one customer.         |
| **Quote**           | Server-computed, expiring price breakdown for a cart+address+coupon combo.          |
| **Order**           | The authoritative purchase record with its snapshot of prices and items.            |
| **Order event**     | Append-only lifecycle record (state transitions, actors, reasons).                  |
| **Assignment**      | A rider's claim on an order delivery; claimed atomically.                           |
| **GOV**             | Gross order value — sum of item subtotals + fees for qualifying orders; NOT payout. |
| **Settlement**      | Platform↔restaurant money statement (commission, deductions, payout status).        |
| **Outbox event**    | Transactionally-committed row that a dispatcher turns into notifications/jobs.      |
| **Idempotency key** | Client-supplied key that makes retried writes (checkout/payment) single-effect.     |
| **Demo mode**       | Local environment with synthetic data and clearly labelled simulated providers.     |

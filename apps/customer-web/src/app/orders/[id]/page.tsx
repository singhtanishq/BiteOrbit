"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  cancelOrder,
  friendlyError,
  getOrder,
  OrderDetail,
  requestRefund,
  simulatePayment,
} from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, EmptyState, Skeleton, cx, formatINR } from "@biteorbit/ui";
import { StatusChip } from "@biteorbit/ui";
import { PaymentChip } from "../page";

const TIMELINE_ORDER = [
  "placed",
  "restaurant_confirmed",
  "preparing",
  "ready_for_pickup",
  "rider_assigned",
  "picked_up",
  "on_the_way",
  "delivered",
];

const TIMELINE_LABELS: Record<string, string> = {
  placed: "Order placed",
  restaurant_confirmed: "Restaurant confirmed",
  preparing: "In the kitchen",
  ready_for_pickup: "Ready for pickup",
  rider_assigned: "Rider assigned",
  picked_up: "Picked up",
  on_the_way: "On the way",
  delivered: "Delivered",
};

export default function OrderDetailPage() {
  const params = useParams<{ id: string }>();
  const id = params.id;
  const [order, setOrder] = useState<OrderDetail | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [cancelOpen, setCancelOpen] = useState(false);
  const [reason, setReason] = useState("");

  const load = useCallback(async () => {
    try {
      setOrder(await getOrder(id));
    } catch (err) {
      setError(friendlyError(err));
    }
  }, [id]);

  useEffect(() => {
    load();
    // polling fallback for live updates (SSE arrives with the courier milestone)
    const timer = setInterval(load, 10_000);
    return () => clearInterval(timer);
  }, [load]);

  const payNow = async (outcome: "success" | "failure") => {
    if (!order) return;
    setBusy(true);
    setNotice("");
    try {
      await simulatePayment(order.payment.payment.id, outcome);
      await load();
      setNotice(
        outcome === "success"
          ? "Payment captured via the simulated gateway (signed webhook path)."
          : "Payment failed (simulated). You can retry below.",
      );
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const doCancel = async () => {
    if (!order || reason.trim().length < 4) return;
    setBusy(true);
    try {
      const res = await cancelOrder(order.id, reason.trim());
      setCancelOpen(false);
      setNotice(
        res.refund_initiated
          ? "Order cancelled — your payment was refunded (simulated)."
          : "Order cancelled.",
      );
      await load();
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const doRefund = async () => {
    if (!order) return;
    setBusy(true);
    try {
      await requestRefund(order.id, "Demo refund request from order page");
      await load();
      setNotice("Refund processed by the simulated provider.");
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  if (error) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-10">
        <EmptyState
          icon="⚠️"
          title="Couldn't load this order"
          description={error}
          action={
            <Link href="/orders">
              <Button variant="secondary">Back to orders</Button>
            </Link>
          }
        />
      </main>
    );
  }

  if (!order) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-10">
        <Skeleton className="h-24 w-full" />
        <Skeleton className="mt-3 h-64 w-full" />
      </main>
    );
  }

  const bd = order.breakdown;
  const pay = order.payment.payment;
  const onlinePending = ["pending"].includes(pay.status) && pay.method !== "cod";
  const isTerminal = ["delivered", "rejected", "cancelled", "delivery_failed"].includes(order.status);
  const canRefund = pay.status === "captured" && ["rejected", "cancelled", "delivery_failed", "delivered"].includes(order.status);

  // timeline states derived from events
  const reached = new Set(order.events.map((e) => e.to_state).filter(Boolean));
  const currentIdx = order.status === "pending_payment" ? -1 : TIMELINE_ORDER.indexOf(order.status);

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-extrabold">{order.reference}</h1>
          <p className="mt-1 text-sm text-ink-600">
            <Link href={`/restaurants/${order.outlet.id}`} className="font-semibold text-coral-600 hover:underline">
              {order.outlet.name}
            </Link>{" "}
            · {order.address}
          </p>
        </div>
        <div className="text-right">
          <StatusChip status={order.status} />
          <p className="mt-1 text-xs text-ink-500">
            ETA {order.eta_min_minutes}–{order.eta_max_minutes} min (demo estimate)
          </p>
        </div>
      </div>

      {notice ? (
        <p className="mt-4 rounded-lg bg-leaf-50 px-4 py-3 text-sm font-medium text-leaf-700" role="status">
          {notice}
        </p>
      ) : null}

      {/* payment panel */}
      <Card className="mt-6">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Payment</CardTitle>
            <PaymentChip status={pay.status} />
          </div>
        </CardHeader>
        <CardBody>
          <p className="text-sm text-ink-600">
            {pay.method === "cod"
              ? "Cash on delivery — pay the rider at your door."
              : `Simulated ${pay.method.replace("mock_", "").toUpperCase()} · ${formatINR(pay.amount_paise)}`}
          </p>
          {pay.failure_reason ? (
            <p className="mt-1 text-xs font-medium text-berry-600">{pay.failure_reason}</p>
          ) : null}
          {onlinePending ? (
            <div className="mt-3 flex flex-wrap gap-2">
              <Button onClick={() => payNow("success")} loading={busy}>
                Pay now (simulated)
              </Button>
              <Button variant="secondary" onClick={() => payNow("failure")} loading={busy}>
                Simulate failure (demo)
              </Button>
            </div>
          ) : null}
          {canRefund ? (
            <div className="mt-3">
              <Button variant="secondary" size="sm" onClick={doRefund} loading={busy}>
                Request refund (simulated)
              </Button>
            </div>
          ) : null}
          {order.refunds.length > 0 ? (
            <ul className="mt-3 space-y-1 text-xs text-ink-500">
              {order.refunds.map((rf) => (
                <li key={rf.id}>
                  Refund {formatINR(rf.amount_paise)} · {rf.status} · {rf.reason}
                </li>
              ))}
            </ul>
          ) : null}
        </CardBody>
      </Card>

      {/* live tracking / timeline */}
      <Card className="mt-4">
        <CardHeader>
          <CardTitle>{isTerminal ? "Order summary" : "Live status"}</CardTitle>
        </CardHeader>
        <CardBody>
          {!isTerminal && order.status !== "pending_payment" ? (
            <ol className="space-y-2.5" aria-label="Order progress">
              {TIMELINE_ORDER.map((state, i) => {
                const done = reached.has(state) || i < currentIdx;
                const current = state === order.status;
                if (currentIdx < i) return null;
                return (
                  <li key={state} className="flex items-center gap-3 text-sm">
                    <span
                      aria-hidden="true"
                      className={cx(
                        "flex h-6 w-6 items-center justify-center rounded-full border-2 text-xs font-bold",
                        current
                          ? "border-coral-500 bg-coral-500 text-white animate-pulse"
                          : done
                            ? "border-leaf-600 bg-leaf-600 text-white"
                            : "border-ink-200 text-ink-400",
                      )}
                    >
                      {done && !current ? "✓" : i + 1}
                    </span>
                    <span className={cx("font-semibold", current ? "text-coral-600" : done ? "text-ink-800" : "text-ink-400")}>
                      {TIMELINE_LABELS[state]}
                    </span>
                    {current ? <Badge tone="coral">now</Badge> : null}
                  </li>
                );
              })}
            </ol>
          ) : null}
          {order.status === "pending_payment" ? (
            <p className="text-sm text-ink-600">
              Waiting for payment confirmation (simulated gateway).
            </p>
          ) : null}
          {order.rider?.name ? (
            <p className="mt-4 rounded-lg bg-cream-100 px-4 py-3 text-sm text-ink-700">
              🛵 Rider: <strong>{order.rider.name}</strong>
              <span className="text-ink-400"> (demo identity — no live GPS)</span>
            </p>
          ) : null}
          <details className="mt-4 text-xs text-ink-500">
            <summary className="cursor-pointer font-semibold text-ink-700">Full event history</summary>
            <ul className="mt-2 space-y-1">
              {order.events.map((e) => (
                <li key={e.id}>
                  {new Date(e.created_at).toLocaleTimeString("en-IN")} · {e.actor_type} ·{" "}
                  {e.to_state ?? e.event_type}
                  {e.reason ? ` — ${e.reason}` : ""}
                </li>
              ))}
            </ul>
          </details>
        </CardBody>
      </Card>

      {/* items */}
      <Card className="mt-4">
        <CardHeader>
          <CardTitle>Items</CardTitle>
        </CardHeader>
        <CardBody className="px-0">
          <ul className="divide-y divide-ink-100">
            {order.items.map((it) => (
              <li key={it.id} className="flex items-start justify-between gap-4 px-5 py-3">
                <div>
                  <div className="flex items-center gap-2">
                    <span
                      aria-label={it.is_veg ? "Vegetarian" : "Non-vegetarian"}
                      className={cx(
                        "inline-flex h-3.5 w-3.5 items-center justify-center rounded-[3px] border-2",
                        it.is_veg ? "border-leaf-600" : "border-berry-500",
                      )}
                    >
                      <span className={cx("h-1.5 w-1.5 rounded-full", it.is_veg ? "bg-leaf-600" : "bg-berry-500")} />
                    </span>
                    <span className="text-sm font-bold text-ink-900">{it.name}</span>
                    <span className="text-xs text-ink-400">× {it.quantity}</span>
                  </div>
                  {it.modifiers.length > 0 ? (
                    <p className="mt-0.5 text-xs text-ink-500">
                      {it.modifiers.map((m) => `${m.group}: ${m.option}`).join(" · ")}
                    </p>
                  ) : null}
                  {it.special_instructions ? (
                    <p className="mt-0.5 text-xs italic text-ink-500">“{it.special_instructions}”</p>
                  ) : null}
                </div>
                <span className="text-sm font-bold">{formatINR(it.item_total_paise)}</span>
              </li>
            ))}
          </ul>
          <dl className="mt-4 space-y-1.5 border-t border-ink-100 px-5 pt-4 text-sm">
            <SummaryRow label="Item subtotal" value={formatINR(bd.items_subtotal_paise)} />
            {bd.coupon_discount_paise > 0 ? (
              <SummaryRow label="Coupon discount" value={`−${formatINR(bd.coupon_discount_paise)}`} tone="leaf" />
            ) : null}
            <SummaryRow label="Packaging" value={formatINR(bd.packaging_fee_paise)} />
            <SummaryRow label="Delivery" value={formatINR(bd.delivery_fee_paise)} />
            <SummaryRow label="Platform fee" value={formatINR(bd.platform_fee_paise)} />
            {bd.small_order_fee_paise > 0 ? (
              <SummaryRow label="Small-order fee" value={formatINR(bd.small_order_fee_paise)} />
            ) : null}
            {bd.tip_paise > 0 ? <SummaryRow label="Tip" value={formatINR(bd.tip_paise)} /> : null}
            <SummaryRow label="GST" value={formatINR(bd.tax_paise)} />
            <div className="flex justify-between border-t border-ink-100 pt-2">
              <dt className="font-extrabold">Total (frozen at order time)</dt>
              <dd className="font-extrabold">{formatINR(bd.grand_total_paise)}</dd>
            </div>
            <p className="text-[11px] text-ink-400">
              Price snapshot from pricing rule v{bd.pricing_rule_version} — never recalculated later.
            </p>
          </dl>
        </CardBody>
      </Card>

      {/* cancel */}
      {order.can_cancel ? (
        <Card className="mt-4">
          <CardBody>
            {!cancelOpen ? (
              <Button variant="danger" size="sm" onClick={() => setCancelOpen(true)}>
                Cancel order
              </Button>
            ) : (
              <div>
                <label htmlFor="cancel-reason" className="text-sm font-semibold text-ink-800">
                  Why are you cancelling?
                </label>
                <input
                  id="cancel-reason"
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  maxLength={300}
                  className="mt-1.5 h-10 w-full rounded-md border border-ink-200 px-3 text-sm"
                  placeholder="e.g., Ordered the wrong outlet by mistake"
                />
                <div className="mt-3 flex gap-2">
                  <Button variant="danger" size="sm" onClick={doCancel} loading={busy} disabled={reason.trim().length < 4}>
                    Confirm cancellation
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => setCancelOpen(false)}>
                    Keep order
                  </Button>
                </div>
                <p className="mt-2 text-xs text-ink-400">
                  Cancellation is allowed until pickup. Captured online payments are refunded
                  (simulated locally).
                </p>
              </div>
            )}
          </CardBody>
        </Card>
      ) : null}
    </main>
  );
}

function SummaryRow({ label, value, tone }: { label: string; value: string; tone?: "leaf" }) {
  return (
    <div className="flex justify-between">
      <dt className="text-ink-600">{label}</dt>
      <dd className={cx("font-semibold", tone === "leaf" ? "text-leaf-700" : "text-ink-900")}>{value}</dd>
    </div>
  );
}

"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { getAssignment, listOffers, riderEarnings, updateAssignmentState, CourierEarnings, CourierOffer, CourierAssignment } from "@/lib/courier";
import { RiderShell, useRiderScope } from "@/components/RiderShell";
import { Badge, Button, Card, CardBody, EmptyState, Skeleton, cx, formatINR } from "@biteorbit/ui";

const STATE_LABELS: Record<string, string> = {
  accepted: "Offer accepted",
  arriving_at_restaurant: "Arriving at outlet",
  arrived_at_restaurant: "At the outlet",
  picked_up: "Order picked up",
  on_the_way: "On the way to customer",
  delivered: "Delivered",
  delivery_failed: "Delivery failed",
};

const NEXT_LABELS: Record<string, { label: string; tone: "primary" | "success" | "danger" }> = {
  arriving_at_restaurant: { label: "On my way to outlet", tone: "primary" },
  arrived_at_restaurant: { label: "I've arrived", tone: "primary" },
  picked_up: { label: "Picked up order", tone: "primary" },
  on_the_way: { label: "Heading to customer", tone: "primary" },
  delivered: { label: "Complete delivery", tone: "success" },
  delivery_failed: { label: "Report failure…", tone: "danger" },
};

export default function RiderDashboardPage() {
  const { profile, ready, toggle } = useRiderScope();
  const [offers, setOffers] = useState<CourierOffer[] | null>(null);
  const [active, setActive] = useState<CourierAssignment | null>(null);
  const [earnings, setEarnings] = useState<CourierEarnings | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [failOpen, setFailOpen] = useState(false);
  const [failReason, setFailReason] = useState("");
  const [otp, setOtp] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      const [o, e] = await Promise.all([listOffers(), riderEarnings()]);
      setOffers(o);
      setEarnings(e);
      // find the current active assignment (accepted/picked_up/on_the_way)
      const first = o[0];
      if (first) {
        setActive(await getAssignment(first.assignment_id));
      } else {
        setActive(null);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (!ready) return;
    load();
    const timer = setInterval(load, 6_000);
    return () => clearInterval(timer);
  }, [ready, load]);

  const advance = async (state: string, extra: { otp?: string; reason?: string } = {}) => {
    if (!active) return;
    setBusy(true);
    setError("");
    try {
      await updateAssignmentState(active.id, state, extra);
      setOtp("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Transition failed");
    } finally {
      setBusy(false);
    }
  };

  if (!ready) {
    return <main className="mx-auto max-w-md px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  const inFlight = active && ["accepted", "arriving_at_restaurant", "arrived_at_restaurant", "picked_up", "on_the_way"].includes(active.status);

  return (
    <RiderShell profile={profile} onToggle={toggle} active="today">
      <main className="mt-5">
        <h1 className="text-xl font-extrabold">Today</h1>
        {error ? (
          <p role="alert" className="mt-3 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">
            {error}
          </p>
        ) : null}

        {earnings ? (
          <div className="mt-3 grid grid-cols-3 gap-2">
            <div className="rounded-xl border border-ink-100 bg-white px-4 py-3 text-center">
              <p className="text-[11px] font-semibold uppercase text-ink-500">Deliveries</p>
              <p className="text-xl font-extrabold">{earnings.totals.deliveries}</p>
            </div>
            <div className="rounded-xl border border-ink-100 bg-white px-4 py-3 text-center">
              <p className="text-[11px] font-semibold uppercase text-ink-500">Earnings</p>
              <p className="text-xl font-extrabold">{formatINR(earnings.totals.total_paise, { withDecimals: false })}</p>
            </div>
            <div className="rounded-xl border border-ink-100 bg-white px-4 py-3 text-center">
              <p className="text-[11px] font-semibold uppercase text-ink-500">Tips</p>
              <p className="text-xl font-extrabold">{formatINR(earnings.totals.tip_paise, { withDecimals: false })}</p>
            </div>
          </div>
        ) : null}

        {inFlight && active ? (
          <Card className="mt-4">
            <CardBody>
              <div className="flex items-center justify-between">
                <span className="font-extrabold">{active.reference}</span>
                <Badge tone="coral">{STATE_LABELS[active.status] ?? active.status}</Badge>
              </div>
              <div className="mt-3 text-sm text-ink-700">
                <p><strong>Pickup:</strong> {active.outlet.name}, {active.outlet.locality}</p>
                <p><strong>Drop:</strong> {active.drop.recipient_name} · {active.drop.line1}, {active.drop.locality}</p>
                {active.drop.instructions ? (
                  <p className="mt-1 text-xs italic text-ink-500">“{active.drop.instructions}”</p>
                ) : null}
                <p className="mt-1 text-xs text-ink-500">
                  {active.items_count} item(s) · ~{active.distance_km} km ·{" "}
                  {active.payment_method === "cod" ? "COLLECT CASH" : "prepaid"}
                </p>
                <p className="mt-1 font-bold text-leaf-700">
                  +{formatINR(active.payout_total_paise, { withDecimals: false })}
                </p>
              </div>

              {active.status === "on_the_way" && active.payment_method === "cod" ? (
                <p className="mt-3 rounded bg-marigold-50 px-3 py-2 text-xs font-bold text-ink-800">
                  Collect {formatINR(active.payout_total_paise)} at the door (demo cash).
                </p>
              ) : null}

              {active.allowed_next_states.includes("delivered") ? (
                <div className="mt-3">
                  <label htmlFor="delivery-otp" className="text-xs font-semibold text-ink-700">
                    Customer&apos;s 4-digit code
                  </label>
                  <input
                    id="delivery-otp"
                    value={otp}
                    onChange={(e) => setOtp(e.target.value.replace(/\D/g, "").slice(0, 4))}
                    inputMode="numeric"
                    aria-label="Delivery confirmation code"
                    className="mt-1 h-11 w-28 rounded-lg border border-ink-200 text-center text-lg font-bold tracking-widest"
                  />
                </div>
              ) : null}

              <div className="mt-3 flex flex-wrap gap-2">
                {active.allowed_next_states.map((s) => {
                  const meta = NEXT_LABELS[s];
                  if (!meta) return null;
                  if (s === "delivery_failed") {
                    return (
                      <Button key={s} variant="ghost" size="sm" onClick={() => setFailOpen(true)}>
                        {meta.label}
                      </Button>
                    );
                  }
                  return (
                    <Button
                      key={s}
                      variant={meta.tone === "success" ? "success" : "primary"}
                      onClick={() => advance(s, { otp: otp || undefined })}
                      loading={busy}
                    >
                      {meta.label}
                    </Button>
                  );
                })}
              </div>
            </CardBody>
          </Card>
        ) : (
          <Card className="mt-4">
            <CardBody className="text-center">
              {offers && offers.length > 0 ? (
                <>
                  <p className="text-sm font-semibold text-ink-800">You have {offers.length} offer(s) waiting!</p>
                  <Link href="/offers">
                    <Button className="mt-3">Review offers</Button>
                  </Link>
                </>
              ) : (
                <EmptyState
                  icon={profile?.availability === "online" ? "🛵" : "😴"}
                  title={profile?.availability === "online" ? "Waiting for offers…" : "You're offline"}
                  description={
                    profile?.availability === "online"
                      ? "Offers appear here when an outlet hands an order to BiteOrbit (demo dispatch runs automatically)."
                      : "Go online from the switch above to start receiving delivery offers."
                  }
                />
              )}
            </CardBody>
          </Card>
        )}

        {earnings && earnings.days.length > 0 ? (
          <div className="mt-4">
            <h2 className="text-sm font-bold text-ink-800">Recent days</h2>
            <ul className="mt-2 space-y-2">
              {earnings.days.slice(0, 5).map((d) => (
                <li key={d.day} className={cx("flex items-center justify-between rounded-lg border border-ink-100 bg-white px-4 py-2.5 text-sm")}>
                  <span>{d.day}</span>
                  <span className="text-ink-500">
                    {d.deliveries} runs · <strong>{formatINR(d.total_paise, { withDecimals: false })}</strong>
                  </span>
                </li>
              ))}
            </ul>
            <Link href="/earnings" className="mt-2 block text-center text-xs font-bold text-coral-600">
              Full earnings →
            </Link>
          </div>
        ) : null}

        {failOpen && active ? (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4" role="dialog" aria-modal="true">
            <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
              <h2 className="text-lg font-extrabold">Report delivery failure</h2>
              <p className="mt-1 text-xs text-ink-500">
                The platform refunds the customer (prepaid) — this is recorded against your run.
              </p>
              <textarea
                value={failReason}
                onChange={(e) => setFailReason(e.target.value)}
                rows={3}
                maxLength={300}
                aria-label="Failure reason"
                placeholder="e.g., Customer unreachable at address…"
                className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
              />
              <div className="mt-4 flex justify-end gap-2">
                <Button variant="ghost" onClick={() => setFailOpen(false)}>
                  Back
                </Button>
                <Button
                  variant="danger"
                  onClick={async () => {
                    await advance("delivery_failed", { reason: failReason.trim() || "Delivery failed (demo)" });
                    setFailOpen(false);
                    setFailReason("");
                  }}
                  disabled={failReason.trim().length < 4}
                  loading={busy}
                >
                  Report failure
                </Button>
              </div>
            </div>
          </div>
        ) : null}
      </main>
    </RiderShell>
  );
}

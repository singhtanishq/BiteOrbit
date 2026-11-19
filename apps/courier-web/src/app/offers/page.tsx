"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { acceptOffer, listOffers, CourierOffer } from "@/lib/courier";
import { RiderShell, useRiderScope } from "@/components/RiderShell";
import { Button, Card, CardBody, EmptyState, Skeleton, cx, formatINR } from "@biteorbit/ui";

export default function OffersPage() {
  const { profile, ready, toggle } = useRiderScope();
  const [offers, setOffers] = useState<CourierOffer[] | null>(null);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [accepted, setAccepted] = useState<{ id: string; orderId: string } | null>(null);

  const load = useCallback(async () => {
    if (profile?.availability !== "online") {
      setOffers([]);
      return;
    }
    try {
      setOffers(await listOffers());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load offers");
    }
  }, [profile?.availability]);

  useEffect(() => {
    if (!ready) return;
    load();
    const timer = setInterval(load, 5_000);
    return () => clearInterval(timer);
  }, [ready, load]);

  const accept = async (offer: CourierOffer) => {
    setBusyId(offer.assignment_id);
    setError("");
    try {
      const res = await acceptOffer(offer.assignment_id);
      setAccepted({ id: offer.assignment_id, orderId: res.order_id });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Accept failed");
      await load();
    } finally {
      setBusyId("");
    }
  };

  if (!ready) {
    return <main className="mx-auto max-w-md px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  return (
    <RiderShell profile={profile} onToggle={toggle} active="offers">
      <main className="mt-5">
        <h1 className="text-xl font-extrabold">Delivery offers</h1>
        {profile && profile.availability !== "online" ? (
          <p className="mt-3 rounded-lg bg-marigold-50 px-4 py-3 text-sm text-ink-700">
            Go ONLINE (top-right) to start receiving offers.
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="mt-3 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">
            {error}
          </p>
        ) : null}

        {accepted ? (
          <div className="mt-4 rounded-xl border border-leaf-100 bg-leaf-50 px-5 py-4 text-sm">
            <p className="font-bold text-leaf-700">Offer accepted!</p>
            <p className="mt-1 text-ink-600">
              Open the delivery flow to start pickup:{" "}
              <Link href={`/delivery/${accepted.id}`} className="font-bold text-coral-600 hover:underline">
                Go to delivery →
              </Link>
            </p>
          </div>
        ) : null}

        {!offers ? (
          <div className="mt-4 space-y-3">
            <Skeleton className="h-28 w-full" />
          </div>
        ) : offers.length === 0 ? (
          <EmptyState
            icon="🛵"
            title="No offers right now"
            description="Stay online — new offers appear automatically when nearby outlets hand orders to BiteOrbit (demo dispatch)."
            className="mt-4"
          />
        ) : (
          <ul className="mt-4 space-y-3">
            {offers.map((o) => (
              <li key={o.assignment_id}>
                <Card className={cx(o.expires_in_sec < 60 && "ring-2 ring-berry-100")}>
                  <CardBody>
                    <div className="flex items-center justify-between">
                      <span className="font-bold">{o.reference}</span>
                      <span
                        className={cx(
                          "text-xs font-bold",
                          o.expires_in_sec < 60 ? "text-berry-600" : "text-ink-500",
                        )}
                      >
                        expires in {Math.max(0, o.expires_in_sec)}s
                      </span>
                    </div>
                    <p className="mt-1 text-sm text-ink-600">
                      🍳 {o.outlet_name} ({o.outlet_locality})
                    </p>
                    <p className="text-sm text-ink-600">🏠 drop-off near {o.drop_locality}</p>
                    <p className="mt-1 text-xs text-ink-500">
                      {o.items_count} item(s) · ~{o.distance_km} km (estimate) · prepay/cash
                      confirmed after acceptance
                    </p>
                    <div className="mt-2 flex items-center justify-between">
                      <span className="text-base font-extrabold text-leaf-700">
                        +{formatINR(o.payout_total_paise, { withDecimals: false })}
                      </span>
                      <Button onClick={() => accept(o)} loading={busyId === o.assignment_id}>
                        Accept
                      </Button>
                    </div>
                  </CardBody>
                </Card>
              </li>
            ))}
          </ul>
        )}
      </main>
    </RiderShell>
  );
}

"use client";

import { useCallback, useEffect, useState } from "react";
import { riderEarnings, CourierEarnings } from "@/lib/courier";
import { RiderShell, useRiderScope } from "@/components/RiderShell";
import { Badge, Card, CardBody, CardHeader, CardTitle, EmptyState, Skeleton, formatINR } from "@biteorbit/ui";

export default function EarningsPage() {
  const { profile, ready, toggle } = useRiderScope();
  const [earnings, setEarnings] = useState<CourierEarnings | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      setEarnings(await riderEarnings());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load earnings");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  if (!ready) {
    return <main className="mx-auto max-w-md px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  return (
    <RiderShell profile={profile} onToggle={toggle} active="earnings">
      <main className="mt-5">
        <h1 className="text-xl font-extrabold">Earnings</h1>
        {error ? (
          <p role="alert" className="mt-3 text-sm text-berry-600">{error}</p>
        ) : null}
        {!earnings ? (
          <Skeleton className="mt-4 h-40 w-full" />
        ) : earnings.days.length === 0 ? (
          <EmptyState
            icon="💰"
            title="No earnings yet"
            description="Complete deliveries to build your earnings ledger."
            className="mt-4"
          />
        ) : (
          <>
            <Card className="mt-4">
              <CardHeader>
                <div className="flex items-center justify-between">
                  <CardTitle>All-time (demo ledger)</CardTitle>
                  <Badge tone="leaf">{earnings.totals.status.split(" (")[0]}</Badge>
                </div>
              </CardHeader>
              <CardBody>
                <div className="grid grid-cols-3 gap-2 text-center">
                  <div>
                    <p className="text-[11px] uppercase text-ink-500">Deliveries</p>
                    <p className="text-xl font-extrabold">{earnings.totals.deliveries}</p>
                  </div>
                  <div>
                    <p className="text-[11px] uppercase text-ink-500">Total</p>
                    <p className="text-xl font-extrabold">{formatINR(earnings.totals.total_paise)}</p>
                  </div>
                  <div>
                    <p className="text-[11px] uppercase text-ink-500">Tips</p>
                    <p className="text-xl font-extrabold">{formatINR(earnings.totals.tip_paise)}</p>
                  </div>
                </div>
                <p className="mt-3 text-[11px] text-ink-400">{earnings.demo_note}</p>
              </CardBody>
            </Card>
            <ul className="mt-4 space-y-2">
              {earnings.days.map((d) => (
                <li key={d.day} className="rounded-xl border border-ink-100 bg-white px-4 py-3 shadow-card">
                  <div className="flex items-center justify-between text-sm">
                    <span className="font-bold">{d.day}</span>
                    <span className="font-extrabold">{formatINR(d.total_paise)}</span>
                  </div>
                  <p className="mt-1 text-xs text-ink-500">
                    {d.deliveries} deliveries · base {formatINR(d.base_paise, { withDecimals: false })}
                    {d.incentive_paise > 0 ? ` · incentive ${formatINR(d.incentive_paise, { withDecimals: false })}` : ""}
                    {d.tip_paise > 0 ? ` · tips ${formatINR(d.tip_paise, { withDecimals: false })}` : ""}
                  </p>
                </li>
              ))}
            </ul>
          </>
        )}
      </main>
    </RiderShell>
  );
}

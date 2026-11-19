"use client";

import { useCallback, useEffect, useState } from "react";
import { PortalShell, MetricCard, useOutletScope } from "@/components/PortalNav";
import { portalAnalytics, PortalAnalytics } from "@/lib/portal";
import { Button, Skeleton, formatINR } from "@biteorbit/ui";

export default function DashboardPage() {
  const { outlets, outlet, outletId, setOutletId, ready } = useOutletScope();
  const [analytics, setAnalytics] = useState<PortalAnalytics | null>(null);
  const [days, setDays] = useState(1);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    if (!outletId) return;
    setError("");
    try {
      setAnalytics(await portalAnalytics(outletId, days));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load analytics");
    }
  }, [outletId, days]);

  useEffect(() => {
    load();
    const timer = setInterval(load, 30_000); // data freshness tick
    return () => clearInterval(timer);
  }, [load]);

  if (!ready) {
    return <main className="mx-auto max-w-6xl px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  return (
    <PortalShell outletId={outletId} outlets={outlets ?? []} onOutletChange={setOutletId} active="dashboard">
      <main className="mt-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-extrabold">{outlet?.name ?? "Outlet"} dashboard</h1>
            <p className="text-xs text-ink-500">
              Data as of {analytics ? new Date(analytics.data_as_of).toLocaleTimeString("en-IN") : "—"} · auto-refreshes
              every 30s
            </p>
          </div>
          <div className="flex gap-1" role="group" aria-label="Range">
            {[
              [1, "Today"],
              [7, "7 days"],
              [30, "30 days"],
            ].map(([d, label]) => (
              <button
                key={label}
                onClick={() => setDays(d as number)}
                aria-pressed={days === d}
                className={
                  days === d
                    ? "rounded-full bg-coral-500 px-3 py-1.5 text-xs font-bold text-white"
                    : "rounded-full bg-cream-100 px-3 py-1.5 text-xs font-bold text-ink-600 hover:bg-cream-200"
                }
              >
                {label}
              </button>
            ))}
            <Button size="sm" variant="secondary" onClick={load}>Refresh</Button>
          </div>
        </div>

        {error ? (
          <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">
            {error}
          </p>
        ) : null}

        {!analytics ? (
          <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            {Array.from({ length: 4 }).map((_, i) => (
              <Skeleton key={i} className="h-28 w-full" />
            ))}
          </div>
        ) : (
          <>
            <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
              <MetricCard label="Orders (excl. unpaid)" value={String(analytics.orders)} sub="placed + beyond" tone="coral" />
              <MetricCard label="Delivered" value={String(analytics.delivered)} sub="completed orders" tone="leaf" />
              <MetricCard
                label="Gross order value"
                value={formatINR(analytics.gov_paise, { withDecimals: false })}
                sub="delivered orders only — not payout"
              />
              <MetricCard
                label="Cancellation rate"
                value={`${(analytics.cancel_rate_bp / 100).toFixed(1)}%`}
                sub={`${analytics.cancelled + analytics.rejected} cancelled + rejected`}
              />
              <MetricCard label="Net sales" value={formatINR(analytics.net_sales_paise, { withDecimals: false })} sub="items − discounts (delivered)" />
              <MetricCard label="Average order value" value={formatINR(analytics.aov_paise)} sub="net sales / delivered" />
              <MetricCard label="Avg prep time" value={`${Math.round(analytics.avg_prep_minutes)} min`} sub="kitchen → ready (delivered)" />
              <MetricCard label="Min order" value={outlet ? formatINR(outlet.min_order_paise, { withDecimals: false }) : "—"} sub={`packaging ${outlet ? formatINR(outlet.packaging_fee_paise, { withDecimals: false }) : "—"}`} />
            </div>

            <div className="mt-8 grid gap-4 lg:grid-cols-2">
              <div className="rounded-xl border border-ink-100 bg-white px-5 py-4 shadow-card">
                <h2 className="text-base font-bold">Popular items (delivered)</h2>
                {analytics.top_items.length === 0 ? (
                  <p className="mt-2 text-sm text-ink-500">No delivered orders in this range yet.</p>
                ) : (
                  <ul className="mt-3 space-y-2">
                    {analytics.top_items.map((t) => (
                      <li key={t.name} className="flex items-center justify-between text-sm">
                        <span className="font-semibold text-ink-800">{t.name}</span>
                        <span className="text-ink-500">
                          {t.qty} sold · {formatINR(t.revenue_paise, { withDecimals: false })}
                        </span>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <div className="rounded-xl border border-ink-100 bg-cream-100 px-5 py-4">
                <h2 className="text-base font-bold">How to read these numbers</h2>
                <ul className="mt-2 space-y-1.5 text-xs text-ink-600">
                  <li><strong>Orders</strong> counts everything except unpaid (pending_payment) carts.</li>
                  <li><strong>Gross order value</strong> sums grand totals of DELIVERED orders only. It is not your payout.</li>
                  <li><strong>Net sales</strong> = item subtotal − coupon discounts on delivered orders. Commissions/settlement adjustments are separate.</li>
                  <li><strong>Prep time</strong> measures kitchen start → ready for pickup.</li>
                  <li>All figures derive from this outlet&apos;s stored demo orders; nothing is estimated live.</li>
                </ul>
                <p className="mt-3 text-[11px] text-ink-400">{analytics.demo_note}</p>
              </div>
            </div>
          </>
        )}
      </main>
    </PortalShell>
  );
}

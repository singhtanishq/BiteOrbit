"use client";

import { useCallback, useEffect, useState } from "react";
import { adminOverview, AdminOverview } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Skeleton, formatINR } from "@biteorbit/ui";

export default function OverviewPage() {
  const { ready } = useAdminScope();
  const [m, setM] = useState<AdminOverview | null>(null);
  const [days, setDays] = useState(7);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setM(await adminOverview(days));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, [days]);

  useEffect(() => {
    if (ready) load();
    const t = setInterval(load, 30_000);
    return () => clearInterval(t);
  }, [ready, load]);

  return (
    <AdminShell active="overview" title="Platform overview" subtitle="Synthetic demo data — refreshes every 30s.">
      <div className="flex gap-1" role="group" aria-label="Range">
        {[[1, "Today"], [7, "7 days"], [30, "30 days"]].map(([d, label]) => (
          <button
            key={label as string}
            onClick={() => setDays(d as number)}
            aria-pressed={days === d}
            className={
              days === d
                ? "rounded-full bg-ink-900 px-3 py-1.5 text-xs font-bold text-cream-50"
                : "rounded-full bg-cream-100 px-3 py-1.5 text-xs font-bold text-ink-600"
            }
          >
            {label}
          </button>
        ))}
      </div>
      {error ? (
        <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p>
      ) : null}
      {!m ? (
        <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
          {Array.from({ length: 8 }).map((_, i) => <Skeleton key={i} className="h-24" />)}
        </div>
      ) : (
        <>
          <div className="mt-5 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
            <Metric label="Orders (non-unpaid)" value={String(m.orders_total)} />
            <Metric label="Gross order value" value={formatINR(m.gov_paise, { withDecimals: false })} sub="delivered only" />
            <Metric label="Platform fees" value={formatINR(m.platform_fees_paise, { withDecimals: false })} sub="delivered orders — not net revenue" />
            <Metric label="Delivery fees" value={formatINR(m.delivery_fees_paise, { withDecimals: false })} sub="delivered orders" />
            <Metric label="Refunds issued" value={formatINR(m.refunds_paise, { withDecimals: false })} />
            <Metric label="Coupon cost" value={formatINR(m.coupon_cost_paise, { withDecimals: false })} sub="platform-funded discounts" />
            <Metric label="Active outlets" value={String(m.active_outlets)} sub={`${m.pending_applications} applications pending`} />
            <Metric label="Riders online" value={`${m.riders_online}/${m.riders_total}`} sub={`${m.users_total} users · ${m.tickets_open} open tickets`} />
          </div>
          <div className="mt-6 rounded-xl border border-ink-100 bg-white px-5 py-4 shadow-card">
            <h2 className="text-base font-bold">Order status distribution</h2>
            <div className="mt-3 flex flex-wrap gap-2">
              {Object.entries(m.orders_by_status).map(([s, n]) => (
                <span key={s} className="rounded-full bg-cream-100 px-3 py-1 text-xs font-semibold text-ink-700">
                  {s.replace(/_/g, " ")}: <strong>{n}</strong>
                </span>
              ))}
            </div>
            <p className="mt-3 text-[11px] text-ink-400">{m.demo_note}</p>
          </div>
        </>
      )}
    </AdminShell>
  );
}

function Metric({ label, value, sub }: { label: string; value: string; sub?: string }) {
  return (
    <div className="rounded-xl border border-ink-100 bg-white px-5 py-4 shadow-card">
      <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">{label}</p>
      <p className="mt-1 text-2xl font-extrabold">{value}</p>
      {sub ? <p className="mt-0.5 text-xs text-ink-400">{sub}</p> : null}
    </div>
  );
}

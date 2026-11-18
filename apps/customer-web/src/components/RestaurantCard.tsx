"use client";

import Link from "next/link";
import { cx, formatINR } from "@biteorbit/ui";
import type { OutletSummary } from "@biteorbit/api-client";

/** Warm gradient placeholder used until real media is configured. */
function OutletArt({ name, veg }: { name: string; veg: boolean }) {
  const initials = name
    .split(" ")
    .map((w) => w[0])
    .slice(0, 2)
    .join("");
  return (
    <div
      aria-hidden="true"
      className={cx(
        "flex h-36 items-center justify-center rounded-t-xl",
        veg
          ? "bg-gradient-to-br from-leaf-100 to-leaf-50"
          : "bg-gradient-to-br from-coral-100 via-cream-200 to-marigold-50",
      )}
    >
      <span className="text-2xl font-extrabold tracking-tight text-ink-700/60">{initials}</span>
    </div>
  );
}

export function RestaurantCard({ outlet }: { outlet: OutletSummary }) {
  return (
    <Link
      href={`/restaurants/${outlet.id}`}
      className="group block overflow-hidden rounded-xl border border-ink-100 bg-white shadow-card transition-shadow hover:shadow-lift focus-visible:outline-2 focus-visible:outline-coral-500"
    >
      <div className="relative">
        <OutletArt name={outlet.name} veg={outlet.is_pure_veg} />
        {outlet.is_pure_veg ? (
          <span className="absolute left-3 top-3 rounded-full bg-white/95 px-2 py-0.5 text-[11px] font-bold text-leaf-700">
            Pure veg
          </span>
        ) : null}
        {!outlet.is_open ? (
          <span className="absolute inset-0 flex items-center justify-center bg-ink-950/55 text-sm font-semibold text-cream-100">
            Closed · {outlet.closed_reason}
          </span>
        ) : null}
      </div>
      <div className="px-4 pb-4 pt-3">
        <div className="flex items-start justify-between gap-2">
          <h3 className="text-base font-bold text-ink-900 group-hover:text-coral-600">{outlet.name}</h3>
          <span
            className={cx(
              "inline-flex shrink-0 items-center gap-1 rounded-md px-1.5 py-0.5 text-xs font-bold",
              outlet.rating >= 4
                ? "bg-leaf-50 text-leaf-700"
                : outlet.rating > 0
                  ? "bg-marigold-50 text-marigold-600"
                  : "bg-cream-200 text-ink-500",
            )}
          >
            ★ {outlet.rating > 0 ? outlet.rating.toFixed(1) : "New"}
          </span>
        </div>
        <p className="mt-0.5 line-clamp-1 text-xs text-ink-500">{outlet.cuisines.join(" · ")}</p>
        <p className="mt-0.5 text-xs text-ink-500">
          {outlet.locality} · {outlet.city}
          {outlet.distance_km != null ? ` · ~${outlet.distance_km} km` : ""}
        </p>
        <div className="mt-2.5 flex items-center justify-between border-t border-ink-100 pt-2.5 text-xs text-ink-600">
          <span>
            🛵 {outlet.prep_minutes_min}–{outlet.prep_minutes_max} min{" "}
            <span className="text-ink-400">(estimate)</span>
          </span>
          <span>
            {outlet.min_order_paise > 0 ? `${formatINR(outlet.min_order_paise, { withDecimals: false })} min` : "No min"}
            {outlet.review_count > 0 ? ` · ${outlet.review_count} ratings` : ""}
          </span>
        </div>
      </div>
    </Link>
  );
}

export function RestaurantCardSkeleton() {
  return (
    <div className="overflow-hidden rounded-xl border border-ink-100 bg-white" aria-hidden="true">
      <div className="h-36 animate-pulse rounded-t-xl bg-ink-100" />
      <div className="space-y-2 px-4 pb-4 pt-3">
        <div className="h-4 w-2/3 animate-pulse rounded bg-ink-100" />
        <div className="h-3 w-1/2 animate-pulse rounded bg-ink-100" />
        <div className="h-3 w-3/4 animate-pulse rounded bg-ink-100" />
      </div>
    </div>
  );
}

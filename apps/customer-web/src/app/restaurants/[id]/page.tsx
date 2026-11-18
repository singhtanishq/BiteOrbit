"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  friendlyError,
  outletDetail,
  outletMenu,
  OutletDetail,
  OutletMenu,
  MenuItem,
} from "@biteorbit/api-client";
import { Badge, Button, EmptyState, Skeleton, Spinner, cx, formatINR } from "@biteorbit/ui";

const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

export default function RestaurantPage() {
  const params = useParams<{ id: string }>();
  const id = params.id;

  const [detail, setDetail] = useState<OutletDetail | null>(null);
  const [menu, setMenu] = useState<OutletMenu | null>(null);
  const [error, setError] = useState("");
  const [activeCategory, setActiveCategory] = useState("");
  const [query, setQuery] = useState("");
  const [vegOnly, setVegOnly] = useState(false);

  useEffect(() => {
    let alive = true;
    Promise.all([outletDetail(id), outletMenu(id)])
      .then(([d, m]) => {
        if (!alive) return;
        setDetail(d);
        setMenu(m);
        if (m.categories.length > 0) setActiveCategory(m.categories[0]?.id ?? "");
      })
      .catch((err) => {
        if (alive) setError(friendlyError(err));
      });
    return () => {
      alive = false;
    };
  }, [id]);

  const filteredCategories = useMemo(() => {
    if (!menu) return [];
    return menu.categories
      .map((c) => ({
        ...c,
        items: c.items.filter((it) => {
          if (vegOnly && !it.is_veg) return false;
          if (query && !`${it.name} ${it.description}`.toLowerCase().includes(query.toLowerCase()))
            return false;
          return true;
        }),
      }))
      .filter((c) => c.items.length > 0);
  }, [menu, query, vegOnly]);

  if (error) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-10">
        <EmptyState
          icon="⚠️"
          title="Couldn't load this restaurant"
          description={error}
          action={
            <Link href="/">
              <Button variant="secondary">Back to discovery</Button>
            </Link>
          }
        />
      </main>
    );
  }

  if (!detail || !menu) {
    return (
      <main className="mx-auto max-w-4xl px-4 py-10">
        <Skeleton className="h-44 w-full" />
        <div className="mt-4 space-y-3">
          <Skeleton className="h-6 w-1/3" />
          <Skeleton className="h-4 w-2/3" />
        </div>
        <div className="mt-8 flex justify-center">
          <Spinner label="Loading menu" />
        </div>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-5xl px-4 pb-16">
      {/* header */}
      <header
        className={cx(
          "-mx-4 rounded-b-2xl px-4 py-8 sm:mx-0 sm:rounded-2xl",
          detail.is_open ? "bg-coral-50" : "bg-ink-100",
        )}
      >
        <div className="flex flex-wrap items-start justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-2xl font-extrabold tracking-tight text-ink-900 sm:text-3xl">
                {detail.name}
              </h1>
              {detail.is_pure_veg ? <Badge tone="leaf">Pure veg</Badge> : null}
            </div>
            <p className="mt-1 text-sm text-ink-600">{detail.description}</p>
            <p className="mt-1 text-xs text-ink-500">
              {detail.cuisines.join(" · ")} · {detail.locality}, {detail.city}
            </p>
            <p className="mt-1 text-xs text-ink-500">
              {detail.address.line1}, {detail.address.locality} · {detail.address.postal_code}
            </p>
            <p className="mt-2 text-xs text-ink-500">
              Delivery estimate {detail.prep_minutes_min}–{detail.prep_minutes_max} min{" "}
              <span className="text-ink-400">(estimate · simulated)</span>
              {detail.min_order_paise > 0
                ? ` · Min order ${formatINR(detail.min_order_paise, { withDecimals: false })}`
                : ""}
            </p>
          </div>
          <div className="text-right">
            <span
              className={cx(
                "inline-flex items-center gap-1 rounded-lg px-2 py-1 text-sm font-bold",
                detail.rating >= 4 ? "bg-leaf-50 text-leaf-700" : "bg-marigold-50 text-marigold-600",
              )}
            >
              ★ {detail.rating > 0 ? detail.rating.toFixed(1) : "New"}
            </span>
            <p className="mt-1 text-xs text-ink-500">
              {detail.review_count} ratings <span className="text-ink-400">(demo reviews)</span>
            </p>
            <Badge tone={detail.is_open ? "leaf" : "berry"} className="mt-2">
              {detail.is_open ? "Open now" : `Closed · ${detail.closed_reason}`}
            </Badge>
          </div>
        </div>
        {detail.collections.length > 0 ? (
          <div className="mt-4 flex flex-wrap gap-2">
            {detail.collections.map((c) => (
              <Badge key={c} tone="coral">
                {c}
              </Badge>
            ))}
          </div>
        ) : null}
      </header>

      {/* hours */}
      <details className="mt-4 rounded-xl border border-ink-100 bg-white px-4 py-3 text-sm">
        <summary className="cursor-pointer font-semibold text-ink-800">Opening hours</summary>
        <ul className="mt-2 space-y-1 text-ink-600">
          {detail.hours.map((h) => (
            <li key={h.weekday} className="flex justify-between">
              <span>{WEEKDAYS[h.weekday]}</span>
              <span>{h.is_closed ? "Closed" : `${h.opens_at} – ${h.closes_at}`}</span>
            </li>
          ))}
        </ul>
        <p className="mt-2 text-xs text-ink-400">Times shown for the demo timezone (Asia/Kolkata).</p>
      </details>

      {/* menu controls */}
      <section className="mt-6" aria-label="Menu">
        <div className="flex flex-wrap items-center gap-3">
          <h2 className="text-xl font-extrabold text-ink-900">Menu</h2>
          <input
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search in menu…"
            aria-label="Search within menu"
            className="h-9 flex-1 rounded-lg border border-ink-200 px-3 text-sm focus:border-coral-500 focus:outline-2 focus:outline-coral-500"
          />
          <label className="flex cursor-pointer items-center gap-2 text-sm font-semibold text-ink-700">
            <input
              type="checkbox"
              checked={vegOnly}
              onChange={(e) => setVegOnly(e.target.checked)}
              className="h-4 w-4 accent-[color:var(--color-leaf-600)]"
            />
            Veg only
          </label>
        </div>

        {/* category nav */}
        <nav aria-label="Menu categories" className="mt-4 flex gap-2 overflow-x-auto pb-1">
          {menu.categories.map((c) => (
            <a
              key={c.id}
              href={`#cat-${c.id}`}
              onClick={() => setActiveCategory(c.id)}
              className={cx(
                "shrink-0 rounded-full px-3 py-1.5 text-xs font-bold transition-colors",
                activeCategory === c.id
                  ? "bg-coral-500 text-white"
                  : "bg-cream-100 text-ink-700 hover:bg-cream-200",
              )}
            >
              {c.name}
            </a>
          ))}
        </nav>

        {/* item lists */}
        {filteredCategories.length === 0 ? (
          <EmptyState
            icon="🍽️"
            title="No dishes match"
            description="Try clearing the menu search or the veg filter."
            className="mt-6"
          />
        ) : (
          filteredCategories.map((c) => (
            <div key={c.id} id={`cat-${c.id}`} className="mt-8 scroll-mt-20">
              <h3 className="text-lg font-extrabold text-ink-900">
                {c.name}
                <span className="ml-2 text-sm font-medium text-ink-400">({c.items.length})</span>
              </h3>
              <ul className="mt-3 divide-y divide-ink-100 overflow-hidden rounded-xl border border-ink-100 bg-white">
                {c.items.map((it) => (
                  <MenuRow key={it.id} item={it} />
                ))}
              </ul>
            </div>
          ))
        )}
      </section>
    </main>
  );
}

function VegDot({ veg }: { veg: boolean }) {
  return (
    <span
      aria-label={veg ? "Vegetarian" : "Non-vegetarian"}
      title={veg ? "Vegetarian" : "Non-vegetarian"}
      className={cx(
        "inline-flex h-4 w-4 shrink-0 items-center justify-center rounded-[4px] border-2",
        veg ? "border-leaf-600" : "border-berry-500",
      )}
    >
      <span className={cx("h-2 w-2 rounded-full", veg ? "bg-leaf-600" : "bg-berry-500")} />
    </span>
  );
}

function MenuRow({ item }: { item: MenuItem }) {
  const [expanded, setExpanded] = useState(false);
  const hasMods = item.modifier_groups.length > 0;

  return (
    <li className="flex items-start justify-between gap-4 px-4 py-4">
      <div className="min-w-0">
        <div className="flex items-center gap-2">
          <VegDot veg={item.is_veg} />
          <h4 className="truncate text-sm font-bold text-ink-900">{item.name}</h4>
          {item.is_bestseller ? <Badge tone="marigold">Bestseller</Badge> : null}
          {item.is_featured ? <Badge tone="coral">Featured</Badge> : null}
        </div>
        <p className="mt-1 text-sm font-bold text-ink-900">{formatINR(item.price_paise)}</p>
        {item.description ? (
          <p className="mt-1 line-clamp-2 text-xs text-ink-500">{item.description}</p>
        ) : null}
        {item.serving_info ? <p className="mt-0.5 text-xs text-ink-400">{item.serving_info}</p> : null}
        {item.allergen_notes ? (
          <p className="mt-1 text-xs text-ink-500">
            <span className="font-semibold">Contains:</span> {item.allergen_notes}{" "}
            <span className="text-ink-400">(as entered by the restaurant)</span>
          </p>
        ) : null}
        {hasMods ? (
          <button
            onClick={() => setExpanded((v) => !v)}
            aria-expanded={expanded}
            className="mt-1.5 text-xs font-bold text-coral-600 hover:underline"
          >
            {expanded ? "Hide customisations" : `Customisable (${item.modifier_groups.length})`}
          </button>
        ) : null}
        {expanded
          ? item.modifier_groups.map((g) => (
              <div key={g.id} className="mt-2 rounded-lg bg-cream-100 px-3 py-2">
                <p className="text-xs font-bold text-ink-800">
                  {g.name}
                  {g.is_required ? (
                    <span className="ml-1 font-normal text-berry-600">required</span>
                  ) : (
                    <span className="ml-1 font-normal text-ink-500">optional</span>
                  )}
                  <span className="ml-1 font-normal text-ink-400">
                    · pick {g.min_select}
                    {g.max_select > 1 ? `–${g.max_select}` : ""}
                  </span>
                </p>
                <ul className="mt-1 space-y-0.5">
                  {g.options.map((o) => (
                    <li key={o.id} className="flex items-center justify-between text-xs text-ink-600">
                      <span>
                        {o.name}
                        {o.is_default ? <span className="ml-1 text-ink-400">(default)</span> : null}
                      </span>
                      <span>
                        {o.price_delta_paise > 0
                          ? `+${formatINR(o.price_delta_paise, { withDecimals: false })}`
                          : "Free"}
                      </span>
                    </li>
                  ))}
                </ul>
              </div>
            ))
          : null}
        {!item.is_available ? (
          <Badge tone="berry" className="mt-2">
            Currently unavailable
          </Badge>
        ) : null}
      </div>
      <div className="shrink-0 text-right">
        <span
          aria-hidden="true"
          className="flex h-16 w-16 items-center justify-center rounded-lg bg-gradient-to-br from-cream-200 to-cream-100 text-ink-300"
        >
          🍽️
        </span>
        {/* Add-to-cart arrives with the cart milestone (M05) */}
        <span className="mt-2 block text-[11px] font-semibold text-ink-400">cart soon</span>
      </div>
    </li>
  );
}

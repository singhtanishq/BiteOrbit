"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import {
  cuisines as fetchCuisines,
  collections as fetchCollections,
  friendlyError,
  listRestaurants,
  OutletSummary,
  CuisineChip,
} from "@biteorbit/api-client";
import { Badge, Button, EmptyState, Skeleton, cx } from "@biteorbit/ui";
import { RestaurantCard, RestaurantCardSkeleton } from "./RestaurantCard";

const SORTS = [
  { value: "", label: "Relevance" },
  { value: "rating", label: "Rating" },
  { value: "delivery_time", label: "Delivery time" },
  { value: "price_asc", label: "Price: low to high" },
  { value: "price_desc", label: "Price: high to low" },
] as const;

const PRICE_BANDS = [
  { value: "", label: "Any price" },
  { value: "1-1", label: "₹ (budget)" },
  { value: "1-2", label: "₹₹" },
  { value: "1-3", label: "₹₹–₹₹₹" },
  { value: "3-4", label: "₹₹₹+" },
] as const;

/** Client-side discovery grid: reads filters from URL, fetches from the API. */
export function DiscoveryGrid({ initialQuery = "" }: { initialQuery?: string }) {
  const router = useRouter();
  const params = useSearchParams();

  const query = params.get("query") ?? initialQuery;
  const cuisine = params.get("cuisine") ?? "";
  const collection = params.get("collection") ?? "";
  const veg = params.get("veg") ?? "";
  const price = params.get("price") ?? "";
  const sort = params.get("sort") ?? "";

  const [outlets, setOutlets] = useState<OutletSummary[] | null>(null);
  const [meta, setMeta] = useState<{ total?: number } | null>(null);
  const [error, setError] = useState("");
  const [cuisineChips, setCuisineChips] = useState<CuisineChip[]>([]);
  const [collectionsRow, setCollectionsRow] = useState<Array<{ name: string; outlets: number }>>([]);

  const setParam = useCallback(
    (key: string, value: string) => {
      const next = new URLSearchParams(params.toString());
      if (value) next.set(key, value);
      else next.delete(key);
      router.replace(`/?${next.toString()}`, { scroll: false });
    },
    [params, router],
  );

  useEffect(() => {
    let alive = true;
    fetchCuisines()
      .then((rows) => alive && setCuisineChips(rows))
      .catch(() => {});
    fetchCollections()
      .then((rows) => alive && setCollectionsRow(rows))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);

  useEffect(() => {
    let alive = true;
    setOutlets(null);
    setError("");
    const [pMin, pMax] = price ? price.split("-") : ["", ""];
    listRestaurants({
      query: query || undefined,
      cuisine: cuisine || undefined,
      collection: collection || undefined,
      veg: veg || undefined,
      price_min: pMin ? Number(pMin) : undefined,
      price_max: pMax ? Number(pMax) : undefined,
      sort: sort || undefined,
      limit: 50,
    })
      .then((rows) => {
        if (!alive) return;
        setOutlets(rows);
        setMeta({ total: rows.length });
      })
      .catch((err) => {
        if (alive) setError(friendlyError(err));
      });
    return () => {
      alive = false;
    };
  }, [query, cuisine, collection, veg, price, sort]);

  const activeFilters = useMemo(
    () => Boolean(query || cuisine || collection || veg || price || sort),
    [query, cuisine, collection, veg, price, sort],
  );

  return (
    <section aria-label="Restaurants" className="mt-8">
      {/* collections row */}
      {collectionsRow.length > 0 ? (
        <div className="mb-6 flex gap-3 overflow-x-auto pb-1" role="list" aria-label="Curated collections">
          {collectionsRow.map((c) => (
            <button
              key={c.name}
              role="listitem"
              onClick={() => setParam("collection", collection === c.name ? "" : c.name)}
              className={cx(
                "flex shrink-0 flex-col rounded-xl border px-4 py-3 text-left transition-colors",
                collection === c.name
                  ? "border-coral-500 bg-coral-50"
                  : "border-ink-100 bg-white hover:border-coral-200",
              )}
            >
              <span className="text-sm font-bold text-ink-900">{c.name}</span>
              <span className="text-xs text-ink-500">{c.outlets} places (demo)</span>
            </button>
          ))}
        </div>
      ) : null}

      {/* filter bar */}
      <div className="flex flex-wrap items-center gap-2" role="group" aria-label="Filters">
        <button
          onClick={() => setParam("veg", veg ? "" : "1")}
          aria-pressed={Boolean(veg)}
          className={cx(
            "rounded-full border px-3 py-1.5 text-xs font-semibold transition-colors",
            veg ? "border-leaf-600 bg-leaf-50 text-leaf-700" : "border-ink-200 bg-white text-ink-600",
          )}
        >
          <span aria-hidden="true" className="mr-1 inline-block h-2 w-2 rounded-sm border border-leaf-600" />
          Veg
        </button>
        <select
          aria-label="Price band"
          value={price}
          onChange={(e) => setParam("price", e.target.value)}
          className="h-8 rounded-full border border-ink-200 bg-white px-3 text-xs font-semibold text-ink-600"
        >
          {PRICE_BANDS.map((p) => (
            <option key={p.value} value={p.value}>
              {p.label}
            </option>
          ))}
        </select>
        <select
          aria-label="Sort by"
          value={sort}
          onChange={(e) => setParam("sort", e.target.value)}
          className="h-8 rounded-full border border-ink-200 bg-white px-3 text-xs font-semibold text-ink-600"
        >
          {SORTS.map((s) => (
            <option key={s.value} value={s.value}>
              Sort: {s.label}
            </option>
          ))}
        </select>
        {activeFilters ? (
          <Button variant="ghost" size="sm" onClick={() => router.replace("/")}>
            Clear all
          </Button>
        ) : null}
        {meta?.total != null ? (
          <span className="ml-auto text-xs text-ink-500">{meta.total} restaurants in Bitepur (demo)</span>
        ) : null}
      </div>

      {/* cuisine chips */}
      <div className="mt-3 flex flex-wrap gap-2">
        {cuisineChips.map((c) => (
          <button
            key={c.slug}
            onClick={() => setParam("cuisine", cuisine === c.slug ? "" : c.slug)}
            aria-pressed={cuisine === c.slug}
            className={cx(
              "rounded-full border px-3 py-1 text-xs font-semibold transition-colors",
              cuisine === c.slug
                ? "border-coral-500 bg-coral-50 text-coral-700"
                : "border-ink-200 bg-white text-ink-600 hover:border-coral-200",
            )}
          >
            {c.name}
          </button>
        ))}
      </div>

      {/* results */}
      <div className="mt-6">
        {error ? (
          <EmptyState
            icon="⚠️"
            title="Couldn't load restaurants"
            description={error}
            action={<Button onClick={() => router.refresh()}>Try again</Button>}
          />
        ) : outlets === null ? (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {Array.from({ length: 6 }).map((_, i) => (
              <RestaurantCardSkeleton key={i} />
            ))}
          </div>
        ) : outlets.length === 0 ? (
          <EmptyState
            icon="🔍"
            title="No restaurants match"
            description="Try a different search term, clear filters, or explore a cuisine below."
            action={
              <Button variant="secondary" onClick={() => router.replace("/")}>
                Clear search & filters
              </Button>
            }
          />
        ) : (
          <>
            {query ? (
              <p className="mb-3 text-sm text-ink-600">
                Results for <strong>“{query}”</strong> — matching names, cuisines and menu items.
              </p>
            ) : null}
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {outlets.map((o) => (
                <RestaurantCard key={o.id} outlet={o} />
              ))}
            </div>
          </>
        )}
      </div>
    </section>
  );
}

/** Small badge for header of curated section (kept for layout parity). */
export function SectionHeader({ title, sub }: { title: string; sub?: string }) {
  return (
    <div className="mb-4 flex items-end justify-between">
      <div>
        <h2 className="text-xl font-extrabold text-ink-900">{title}</h2>
        {sub ? <p className="text-xs text-ink-500">{sub}</p> : null}
      </div>
      <Badge tone="neutral">demo data</Badge>
    </div>
  );
}

export function OutletTileSkeleton() {
  return <Skeleton className="h-10 w-40" />;
}

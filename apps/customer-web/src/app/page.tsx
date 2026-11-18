"use client";

import { Suspense } from "react";
import { Badge } from "@biteorbit/ui";
import { SearchBar } from "@/components/SearchBar";
import { DiscoveryGrid, SectionHeader } from "@/components/DiscoveryGrid";
import { useSession } from "@/lib/useSession";

/**
 * Customer home: search, location, curated collections and the discovery
 * grid. All restaurant data comes live from the Go API (seeded demo data).
 */
export default function HomePage() {
  const { user } = useSession();

  return (
    <main className="mx-auto max-w-6xl px-4 py-6">
      <section className="rounded-2xl bg-coral-50 px-4 py-8 sm:px-8" aria-label="Welcome">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-ink-800">
            <span aria-hidden="true">📍</span>
            Bitepur <span className="text-ink-500">(demo city)</span>
            <Badge tone="marigold">demo</Badge>
          </div>
          {user ? (
            <span className="text-sm text-ink-600">
              Hi, <strong>{user.name.split(" ")[0]}</strong>
            </span>
          ) : (
            <span className="text-xs text-ink-500">
              Sign-in and ordering arrive in later milestones — browsing works now.
            </span>
          )}
        </div>
        <h1 className="mt-5 text-2xl font-extrabold tracking-tight text-ink-900 sm:text-3xl">
          Great food. Great local places.{" "}
          <span className="text-coral-600">Right to your door.</span>
        </h1>
        <div className="mt-5 max-w-2xl">
          <SearchBar />
        </div>
        <p className="mt-3 text-xs text-ink-500">
          Everything here is a synthetic demo — the city of Bitepur, its restaurants and its
          people don&apos;t exist.
        </p>
      </section>

      <div className="mt-10">
        <SectionHeader
          title="Explore restaurants"
          sub="Filters and sorting are reflected in the URL so you can share a view."
        />
        <Suspense fallback={<GridSkeleton />}>
          <DiscoveryGrid />
        </Suspense>
      </div>
    </main>
  );
}

function GridSkeleton() {
  return (
    <div className="mt-6 grid gap-4 sm:grid-cols-2 lg:grid-cols-3" aria-hidden="true">
      {Array.from({ length: 6 }).map((_, i) => (
        <div key={i} className="h-64 animate-pulse rounded-xl bg-ink-100/60" />
      ))}
    </div>
  );
}

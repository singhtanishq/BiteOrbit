"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { getSession, logout, friendlyError } from "@biteorbit/api-client";
import { Badge, Button, Logo, Select, cx } from "@biteorbit/ui";
import { listPortalOutlets, PortalOutlet } from "@/lib/portal";

/**
 * Shared portal shell: auth guard, outlet picker, nav.
 * Redirects to /login when anonymous or non-staff.
 */
export function useOutletScope() {
  const [outlets, setOutlets] = useState<PortalOutlet[] | null>(null);
  const [outletId, setOutletId] = useState("");
  const [user, setUser] = useState<{ name: string; email: string; roles: string[] } | null>(null);
  const [ready, setReady] = useState(false);
  const router = useRouter();

  useEffect(() => {
    let alive = true;
    (async () => {
      try {
        const u = await getSession();
        if (!alive) return;
        if (!u.roles.some((r) => r.startsWith("restaurant_"))) {
          router.replace("/login");
          return;
        }
        setUser({ name: u.name, email: u.email, roles: u.roles });
        const rows = await listPortalOutlets();
        if (!alive) return;
        setOutlets(rows);
        if (rows.length > 0) setOutletId((prev) => prev || rows[0].id);
        setReady(true);
      } catch (err) {
        if (friendlyError(err).includes("sign in")) {
          router.replace("/login");
        }
        if (alive) setReady(true);
      }
    })();
    return () => {
      alive = false;
    };
  }, [router]);

  const outlet = outlets?.find((o) => o.id === outletId) ?? null;
  return { outlets, outlet, outletId, setOutletId, user, ready };
}

export function PortalShell({
  children,
  outletId,
  outlets,
  onOutletChange,
  active,
}: {
  children: React.ReactNode;
  outletId: string;
  outlets: PortalOutlet[];
  onOutletChange: (id: string) => void;
  active: "dashboard" | "orders" | "menu";
}) {
  const router = useRouter();
  const signOut = useCallback(async () => {
    try {
      await logout();
    } finally {
      router.push("/login");
    }
  }, [router]);

  return (
    <div className="mx-auto max-w-6xl px-4 py-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-4">
          <Logo />
          <nav aria-label="Portal" className="flex gap-1 text-sm font-semibold">
            {(
              [
                ["dashboard", "/dashboard", "Dashboard"],
                ["orders", "/orders", "Orders"],
                ["menu", "/menu", "Menu"],
              ] as const
            ).map(([key, href, label]) => (
              <a
                key={key}
                href={href}
                aria-current={active === key ? "page" : undefined}
                className={cx(
                  "rounded-full px-3 py-1.5",
                  active === key ? "bg-coral-500 text-white" : "text-ink-700 hover:bg-cream-200",
                )}
              >
                {label}
              </a>
            ))}
          </nav>
        </div>
        <div className="flex items-center gap-2">
          <Select
            aria-label="Outlet"
            value={outletId}
            onChange={(e) => onOutletChange(e.target.value)}
            className="w-56"
          >
            {outlets.map((o) => (
              <option key={o.id} value={o.id}>
                {o.name} · {o.locality}
              </option>
            ))}
          </Select>
          <Button size="sm" variant="ghost" onClick={signOut}>
            Sign out
          </Button>
        </div>
      </div>
      {outlets.length === 0 ? (
        <p className="mt-6 rounded-lg bg-marigold-50 px-4 py-3 text-sm text-ink-700">
          Your staff account has no outlets assigned yet. Ask the platform admin (demo) to grant
          access.
        </p>
      ) : (
        children
      )}
    </div>
  );
}

export function MetricCard({
  label,
  value,
  sub,
  tone = "neutral",
}: {
  label: string;
  value: string;
  sub?: string;
  tone?: "neutral" | "coral" | "leaf";
}) {
  return (
    <div
      className={cx(
        "rounded-xl border bg-white px-5 py-4 shadow-card",
        tone === "coral" ? "border-coral-200" : tone === "leaf" ? "border-leaf-100" : "border-ink-100",
      )}
    >
      <p className="text-xs font-semibold uppercase tracking-wide text-ink-500">{label}</p>
      <p className="mt-1 text-2xl font-extrabold text-ink-900">{value}</p>
      {sub ? <p className="mt-0.5 text-xs text-ink-400">{sub}</p> : null}
    </div>
  );
}

export function StaffBadge({ role }: { role?: string }) {
  if (!role) return null;
  return <Badge tone="sky">{role}</Badge>;
}

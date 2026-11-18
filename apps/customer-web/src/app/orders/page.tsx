"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { friendlyError, listOrders, OrderListItem } from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, EmptyState, Skeleton, formatINR } from "@biteorbit/ui";
import { useSession } from "@/lib/useSession";
import { StatusChip } from "@/components/StatusChip";

export default function OrdersPage() {
  const { user, loaded } = useSession();
  const [orders, setOrders] = useState<OrderListItem[] | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    try {
      const rows = await listOrders();
      setOrders(rows);
    } catch (err) {
      if (err instanceof Error && err.message.includes("401")) {
        setOrders([]);
        return;
      }
      setError(friendlyError(err));
      setOrders([]);
    }
  }, []);

  useEffect(() => {
    if (loaded) load();
  }, [loaded, load]);

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Your orders</h1>
      {error ? (
        <p role="alert" className="mt-4 text-sm font-medium text-berry-600">
          {error}
        </p>
      ) : null}

      {!loaded || orders === null ? (
        <div className="mt-6 space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <Skeleton key={i} className="h-24 w-full" />
          ))}
        </div>
      ) : orders.length === 0 ? (
        <EmptyState
          icon="🧾"
          title={user ? "No orders yet" : "Sign in to see your orders"}
          description={
            user
              ? "Place your first order from any Bitepur restaurant (demo)."
              : "Your order history and live tracking live here once you're signed in."
          }
          className="mt-6"
          action={
            <Link href={user ? "/" : "/account"}>
              <Button>{user ? "Browse restaurants" : "Sign in"}</Button>
            </Link>
          }
        />
      ) : (
        <ul className="mt-6 space-y-3">
          {orders.map((o) => (
            <li key={o.id}>
              <Link href={`/orders/${o.id}`} className="block group">
                <Card className="transition-shadow group-hover:shadow-lift">
                  <CardBody>
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <span className="font-bold text-ink-900">{o.reference}</span>
                        <StatusChip status={o.status} />
                      </div>
                      <span className="text-sm font-bold">{formatINR(o.grand_total_paise)}</span>
                    </div>
                    <p className="mt-1 text-sm text-ink-600">{o.outlet_name}</p>
                    <p className="mt-0.5 text-xs text-ink-400">
                      Placed {new Date(o.placed_at).toLocaleString("en-IN")} ·{" "}
                      {o.eta_min_minutes}–{o.eta_max_minutes} min (demo estimate)
                    </p>
                    <div className="mt-2">
                      <PaymentChip status={o.payment_status} />
                    </div>
                  </CardBody>
                </Card>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}

export function PaymentChip({ status }: { status: string }) {
  const map: Record<string, { tone: "leaf" | "marigold" | "berry" | "neutral" | "sky"; label: string }> = {
    captured: { tone: "leaf", label: "Paid" },
    cod_due: { tone: "marigold", label: "Pay on delivery" },
    cod_collected: { tone: "leaf", label: "Collected" },
    pending: { tone: "sky", label: "Payment pending" },
    failed: { tone: "berry", label: "Payment failed" },
    refunded: { tone: "neutral", label: "Refunded" },
    partially_refunded: { tone: "neutral", label: "Partly refunded" },
    cancelled: { tone: "neutral", label: "Cancelled" },
    none: { tone: "neutral", label: "No payment" },
  };
  const info = map[status] ?? { tone: "neutral" as const, label: status };
  return <Badge tone={info.tone}>{info.label}</Badge>;
}

// helper export used by the orders detail page
export function formatPlacedAt(iso: string) {
  return new Date(iso).toLocaleString("en-IN");
}

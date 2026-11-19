"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  listPortalOrders,
  portalTransition,
  PortalOrder,
} from "@/lib/portal";
import { PortalShell, useOutletScope } from "@/components/PortalNav";
import { Button, EmptyState, Skeleton, cx, formatINR } from "@biteorbit/ui";
import { StatusChip } from "@biteorbit/ui";

const FILTERS = [
  { value: "", label: "All" },
  { value: "placed", label: "New" },
  { value: "restaurant_confirmed", label: "Accepted" },
  { value: "preparing", label: "Preparing" },
  { value: "ready_for_pickup", label: "Ready" },
  { value: "delivered", label: "Delivered" },
  { value: "rejected", label: "Rejected" },
  { value: "cancelled", label: "Cancelled" },
] as const;


export default function OrdersQueuePage() {
  const { outlets, outlet, outletId, setOutletId, ready } = useOutletScope();
  const [orders, setOrders] = useState<PortalOrder[] | null>(null);
  const [filter, setFilter] = useState<string>("");
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [rejecting, setRejecting] = useState<PortalOrder | null>(null);
  const [reason, setReason] = useState("");
  const [printing, setPrinting] = useState<PortalOrder | null>(null);

  const load = useCallback(async () => {
    if (!outletId) return;
    setError("");
    try {
      setOrders(await listPortalOrders(outletId, filter));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load orders");
    }
  }, [outletId, filter]);

  useEffect(() => {
    load();
    const timer = setInterval(load, 10_000);
    return () => clearInterval(timer);
  }, [load]);

  const newCount = useMemo(
    () => (orders ?? []).filter((o) => o.status === "placed").length,
    [orders],
  );

  const act = async (order: PortalOrder, action: "accept" | "prepare" | "ready") => {
    setBusyId(order.id);
    setError("");
    try {
      await portalTransition(order.id, action);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusyId("");
    }
  };

  const doReject = async () => {
    if (!rejecting) return;
    setBusyId(rejecting.id);
    setError("");
    try {
      await portalTransition(rejecting.id, "reject", { reason: reason.trim() });
      setRejecting(null);
      setReason("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Reject failed");
    } finally {
      setBusyId("");
    }
  };

  if (!ready) {
    return <main className="mx-auto max-w-6xl px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  return (
    <PortalShell outletId={outletId} outlets={outlets ?? []} onOutletChange={setOutletId} active="orders">
      <main className="mt-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <h1 className="text-2xl font-extrabold">
            Order queue
            {newCount > 0 ? (
              <span className="ml-3 inline-flex h-7 min-w-7 items-center justify-center rounded-full bg-coral-500 px-2 text-sm font-bold text-white" role="status">
                {newCount} new
              </span>
            ) : null}
          </h1>
          <div className="flex flex-wrap gap-1" role="group" aria-label="Status filter">
            {FILTERS.map((f) => (
              <button
                key={f.value}
                onClick={() => setFilter(f.value)}
                aria-pressed={filter === f.value}
                className={cx(
                  "rounded-full px-3 py-1.5 text-xs font-bold",
                  filter === f.value
                    ? "bg-coral-500 text-white"
                    : "bg-cream-100 text-ink-600 hover:bg-cream-200",
                )}
              >
                {f.label}
              </button>
            ))}
          </div>
        </div>

        {error ? (
          <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">
            {error}
          </p>
        ) : null}

        {!orders ? (
          <div className="mt-6 space-y-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-32 w-full" />
            ))}
          </div>
        ) : orders.length === 0 ? (
          <EmptyState
            icon="📋"
            title="No orders in this view"
            description="New orders appear here the moment customers check out."
            className="mt-6"
          />
        ) : (
          <ul className="mt-6 space-y-4">
            {orders.map((o) => (
              <li
                key={o.id}
                className={cx(
                  "rounded-xl border bg-white shadow-card",
                  o.status === "placed" ? "border-coral-300 ring-2 ring-coral-100" : "border-ink-100",
                )}
              >
                <div className="flex flex-wrap items-center justify-between gap-3 border-b border-ink-100 px-5 py-3">
                  <div className="flex items-center gap-3">
                    <span className="font-extrabold">{o.reference}</span>
                    <StatusChip status={o.status} />
                    {o.delivery_status ? (
                      <span className="text-xs text-ink-500">delivery: {o.delivery_status}</span>
                    ) : null}
                  </div>
                  <span className="text-xs text-ink-500">
                    placed {new Date(o.placed_at).toLocaleTimeString("en-IN")} · {formatINR(o.grand_total_paise)}
                  </span>
                </div>
                <div className="grid gap-4 px-5 py-4 sm:grid-cols-[1fr_auto]">
                  <div>
                    <ul className="space-y-1 text-sm">
                      {o.items.map((it, i) => (
                        <li key={i} className="flex justify-between gap-4">
                          <span>
                            <strong>{it.qty}×</strong> {it.name}
                            {it.instructions ? (
                              <span className="block text-xs italic text-ink-500">“{it.instructions}”</span>
                            ) : null}
                          </span>
                          <span className="text-ink-500">{formatINR(it.unit_price_paise * it.qty)}</span>
                        </li>
                      ))}
                    </ul>
                    {o.instructions ? (
                      <p className="mt-2 rounded bg-marigold-50 px-3 py-2 text-xs text-ink-700">
                        Customer note: {o.instructions}
                      </p>
                    ) : null}
                  </div>
                  <div className="flex shrink-0 flex-col gap-2">
                    {o.status === "placed" ? (
                      <>
                        <Button onClick={() => act(o, "accept")} loading={busyId === o.id}>
                          Accept order
                        </Button>
                        <Button variant="danger" size="sm" onClick={() => setRejecting(o)}>
                          Reject…
                        </Button>
                      </>
                    ) : null}
                    {o.status === "restaurant_confirmed" ? (
                      <Button onClick={() => act(o, "prepare")} loading={busyId === o.id}>
                        Start preparing
                      </Button>
                    ) : null}
                    {o.status === "preparing" ? (
                      <Button variant="success" onClick={() => act(o, "ready")} loading={busyId === o.id}>
                        Mark ready for pickup
                      </Button>
                    ) : null}
                    {["restaurant_confirmed", "preparing", "ready_for_pickup"].includes(o.status) ? (
                      <Button variant="ghost" size="sm" onClick={() => setPrinting(o)}>
                        Kitchen ticket
                      </Button>
                    ) : null}
                  </div>
                </div>
              </li>
            ))}
          </ul>
        )}
      </main>

      {/* reject dialog */}
      {rejecting ? (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4"
          role="dialog"
          aria-modal="true"
          aria-label={`Reject ${rejecting.reference}`}
        >
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">Reject {rejecting.reference}</h2>
            <p className="mt-1 text-xs text-ink-500">
              The customer is refunded automatically for captured online payments (demo).
            </p>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              maxLength={300}
              rows={3}
              aria-label="Rejection reason"
              placeholder="e.g., Item unavailable, closing early…"
              className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm focus:border-coral-500 focus:outline-2 focus:outline-coral-500"
            />
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setRejecting(null)}>
                Cancel
              </Button>
              <Button variant="danger" onClick={doReject} loading={busyId === rejecting.id} disabled={reason.trim().length < 4}>
                Reject order
              </Button>
            </div>
          </div>
        </div>
      ) : null}

      {/* kitchen ticket (print view) */}
      {printing ? (
        <div
          className="fixed inset-0 z-50 overflow-auto bg-white p-8"
          role="dialog"
          aria-modal="true"
          aria-label={`Kitchen ticket ${printing.reference}`}
        >
          <div className="mx-auto max-w-md font-mono text-sm text-ink-900">
            <p className="text-center text-lg font-extrabold tracking-widest">KITCHEN TICKET</p>
            <p className="mt-1 text-center">{printing.reference} · {new Date(printing.placed_at).toLocaleTimeString("en-IN")}</p>
            <hr className="my-3 border-ink-900" />
            <ul className="space-y-2">
              {printing.items.map((it, i) => (
                <li key={i} className="text-base">
                  <span className="font-extrabold">{it.qty}×</span> {it.name}
                  {it.instructions ? (
                    <span className="block pl-6 text-xs uppercase">** {it.instructions} **</span>
                  ) : null}
                </li>
              ))}
            </ul>
            <hr className="my-3 border-ink-900" />
            <p className="text-xs">Total: {formatINR(printing.grand_total_paise)}</p>
            {printing.instructions ? <p className="mt-2 text-xs uppercase">Note: {printing.instructions}</p> : null}
            <p className="mt-6 text-center text-xs">{outlet?.name} · BiteOrbit demo ticket</p>
          </div>
          <div className="mx-auto mt-6 flex max-w-md justify-center gap-3 print:hidden">
            <Button onClick={() => window.print()}>Print</Button>
            <Button variant="secondary" onClick={() => setPrinting(null)}>
              Close
            </Button>
          </div>
        </div>
      ) : null}
    </PortalShell>
  );
}

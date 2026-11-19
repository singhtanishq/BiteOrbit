"use client";

import { useCallback, useEffect, useState } from "react";
import { adminCancelOrder, adminOrders, AdminOrderRow } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Button, EmptyState, Skeleton, StatusChip, formatINR } from "@biteorbit/ui";

export default function AdminOrdersPage() {
  const { ready } = useAdminScope();
  const [rows, setRows] = useState<AdminOrderRow[] | null>(null);
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [cancelling, setCancelling] = useState<AdminOrderRow | null>(null);
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setError("");
    try {
      setRows(await adminOrders(query, status));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, [query, status]);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const cancel = async () => {
    if (!cancelling) return;
    setBusy(true);
    try {
      await adminCancelOrder(cancelling.id, reason.trim());
      setCancelling(null);
      setReason("");
      setNotice(`Order ${cancelling.reference} cancelled and refunded (audited).`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Cancel failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <AdminShell
      active="orders"
      title="Orders"
      subtitle="Platform-wide search. Cancellations require a reason and are audited; support agents cannot cancel."
    >
      <div className="flex flex-wrap gap-2">
        <input
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search reference, outlet or customer…"
          aria-label="Search orders"
          className="h-10 flex-1 rounded-lg border border-ink-200 px-3 text-sm"
        />
        <select
          value={status}
          onChange={(e) => setStatus(e.target.value)}
          aria-label="Status filter"
          className="h-10 rounded-lg border border-ink-200 bg-white px-3 text-sm"
        >
          <option value="">Any status</option>
          {["pending_payment", "placed", "restaurant_confirmed", "preparing", "ready_for_pickup", "rider_assigned", "picked_up", "on_the_way", "delivered", "rejected", "cancelled", "delivery_failed"].map((s) => (
            <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
          ))}
        </select>
        <Button onClick={load}>Search</Button>
      </div>
      {notice ? <p role="status" className="mt-4 rounded-lg bg-leaf-50 px-4 py-3 text-sm text-leaf-700">{notice}</p> : null}
      {error ? <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}

      {!rows ? (
        <Skeleton className="mt-6 h-64 w-full" />
      ) : rows.length === 0 ? (
        <EmptyState icon="🔍" title="No orders match" className="mt-6" />
      ) : (
        <div className="mt-6 overflow-x-auto rounded-xl border border-ink-100 bg-white shadow-card">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs uppercase text-ink-500">
                <th className="px-4 py-3">Reference</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Outlet</th>
                <th className="px-4 py-3">Customer</th>
                <th className="px-4 py-3">Total</th>
                <th className="px-4 py-3">Payment</th>
                <th className="px-4 py-3">Placed</th>
                <th className="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink-100">
              {rows.map((o) => (
                <tr key={o.id} className="hover:bg-cream-50">
                  <td className="px-4 py-3 font-bold">{o.reference}</td>
                  <td className="px-4 py-3"><StatusChip status={o.status} /></td>
                  <td className="px-4 py-3">{o.outlet_name}</td>
                  <td className="px-4 py-3 text-ink-500">{o.customer_email}</td>
                  <td className="px-4 py-3 font-semibold">{formatINR(o.grand_total_paise)}</td>
                  <td className="px-4 py-3 text-xs">{o.payment_status.replace(/_/g, " ")}</td>
                  <td className="px-4 py-3 text-xs text-ink-500">{new Date(o.placed_at).toLocaleString("en-IN")}</td>
                  <td className="px-4 py-3">
                    {o.status !== "delivered" && o.status !== "cancelled" && o.status !== "rejected" ? (
                      <Button size="sm" variant="ghost" onClick={() => setCancelling(o)}>
                        Intervene…
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {cancelling ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4" role="dialog" aria-modal="true">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">Cancel {cancelling.reference}</h2>
            <p className="mt-1 text-xs text-ink-500">
              Administrative intervention: the order is cancelled and captured payments refunded.
              This action is recorded in the audit log with your identity.
            </p>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              aria-label="Cancellation reason"
              placeholder="Mandatory reason (shown in the audit log)…"
              className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
            />
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setCancelling(null)}>Back</Button>
              <Button variant="danger" onClick={cancel} disabled={reason.trim().length < 4} loading={busy}>
                Cancel order
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </AdminShell>
  );
}

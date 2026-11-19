"use client";

import { useCallback, useEffect, useState } from "react";
import { adminRiderAction, adminRiders, AdminRiderRow } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Badge, Button, EmptyState, Skeleton } from "@biteorbit/ui";

export default function RidersPage() {
  const { ready } = useAdminScope();
  const [riders, setRiders] = useState<AdminRiderRow[] | null>(null);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [action, setAction] = useState<{ rider: AdminRiderRow; kind: "approve" | "suspend" } | null>(null);
  const [reason, setReason] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setRiders(await adminRiders());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const doAction = async () => {
    if (!action) return;
    setBusyId(action.rider.id);
    try {
      await adminRiderAction(action.rider.id, action.kind, reason.trim());
      setAction(null);
      setReason("");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Action failed");
    } finally {
      setBusyId("");
    }
  };

  return (
    <AdminShell active="riders" title="Delivery partners" subtitle="Approvals and suspensions are audited. Suspended riders go offline immediately.">
      {error ? <p role="alert" className="rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}
      {!riders ? (
        <Skeleton className="mt-4 h-56 w-full" />
      ) : riders.length === 0 ? (
        <EmptyState icon="🛵" title="No riders" className="mt-4" />
      ) : (
        <div className="mt-4 overflow-x-auto rounded-xl border border-ink-100 bg-white shadow-card">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs uppercase text-ink-500">
                <th className="px-4 py-3">Rider</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Availability</th>
                <th className="px-4 py-3">Vehicle</th>
                <th className="px-4 py-3">Deliveries</th>
                <th className="px-4 py-3"></th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink-100">
              {riders.map((r) => (
                <tr key={r.id} className="hover:bg-cream-50">
                  <td className="px-4 py-3">
                    <p className="font-bold">{r.name}</p>
                    <p className="text-xs text-ink-500">{r.email}</p>
                  </td>
                  <td className="px-4 py-3">
                    <Badge tone={r.status === "approved" ? "leaf" : r.status === "suspended" ? "berry" : "marigold"}>
                      {r.status}
                    </Badge>
                  </td>
                  <td className="px-4 py-3">{r.availability}</td>
                  <td className="px-4 py-3">{r.vehicle} {r.vehicle_number}</td>
                  <td className="px-4 py-3">{r.deliveries}</td>
                  <td className="px-4 py-3">
                    {r.status === "approved" ? (
                      <Button size="sm" variant="ghost" onClick={() => setAction({ rider: r, kind: "suspend" })}>
                        Suspend…
                      </Button>
                    ) : (
                      <Button size="sm" variant="ghost" onClick={() => setAction({ rider: r, kind: "approve" })}>
                        Approve…
                      </Button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {action ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4" role="dialog" aria-modal="true">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">
              {action.kind === "approve" ? "Approve" : "Suspend"} {action.rider.name}
            </h2>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              aria-label="Reason"
              placeholder="Mandatory reason (audited)…"
              className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
            />
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setAction(null)}>Back</Button>
              <Button
                variant={action.kind === "suspend" ? "danger" : "primary"}
                onClick={doAction}
                disabled={reason.trim().length < 4}
                loading={busyId === action.rider.id}
              >
                Confirm
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </AdminShell>
  );
}

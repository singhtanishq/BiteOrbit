"use client";

import { useCallback, useEffect, useState } from "react";
import {
  adminApplications,
  adminRestaurants,
  adminReviewApplication,
  AdminApplicationRow,
  AdminOrgRow,
} from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Badge, Button, Card, CardBody, EmptyState, Skeleton, cx } from "@biteorbit/ui";

export default function RestaurantsPage() {
  const { ready } = useAdminScope();
  const [orgs, setOrgs] = useState<AdminOrgRow[] | null>(null);
  const [apps, setApps] = useState<AdminApplicationRow[] | null>(null);
  const [reviewing, setReviewing] = useState<AdminApplicationRow | null>(null);
  const [decision, setDecision] = useState<"approve" | "reject" | "changes_requested">("approve");
  const [note, setNote] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    setError("");
    try {
      const [o, a] = await Promise.all([adminRestaurants(), adminApplications()]);
      setOrgs(o);
      setApps(a);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const review = async () => {
    if (!reviewing) return;
    setBusy(true);
    try {
      await adminReviewApplication(reviewing.id, decision, note.trim());
      setReviewing(null);
      setNote("");
      setNotice(`Application ${decision} (demo simulation — not legal verification).`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Review failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <AdminShell
      active="restaurants"
      title="Restaurants & applications"
      subtitle="Demo approvals are explicit simulations — no legal or licence verification is performed."
    >
      {notice ? <p role="status" className="rounded-lg bg-leaf-50 px-4 py-3 text-sm text-leaf-700">{notice}</p> : null}
      {error ? <p role="alert" className="mt-2 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}

      <h2 className="mt-4 text-base font-bold">Pending applications</h2>
      {!apps ? (
        <Skeleton className="mt-3 h-32 w-full" />
      ) : apps.length === 0 ? (
        <EmptyState icon="📭" title="No applications" className="mt-3" />
      ) : (
        <ul className="mt-3 space-y-3">
          {apps.map((a) => (
            <li key={a.id}>
              <Card>
                <CardBody>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="font-bold">{a.organisation}</span>
                      <Badge tone={a.status === "approved" ? "leaf" : a.status === "rejected" ? "berry" : "marigold"}>
                        {a.status.replace(/_/g, " ")}
                      </Badge>
                    </div>
                    <span className="text-xs text-ink-500">{new Date(a.created_at).toLocaleDateString("en-IN")}</span>
                  </div>
                  <p className="mt-1 text-sm text-ink-600">{a.applicant_note || "(no note)"}</p>
                  {a.decision_note ? (
                    <p className="mt-1 text-xs text-ink-500">Decision: {a.decision_note} — {a.reviewer_email}</p>
                  ) : null}
                  {a.status === "submitted" || a.status === "under_review" ? (
                    <div className="mt-3">
                      <Button size="sm" onClick={() => { setReviewing(a); setDecision("approve"); setNote(""); }}>
                        Review…
                      </Button>
                    </div>
                  ) : null}
                </CardBody>
              </Card>
            </li>
          ))}
        </ul>
      )}

      <h2 className="mt-8 text-base font-bold">Organisations</h2>
      {!orgs ? (
        <Skeleton className="mt-3 h-40 w-full" />
      ) : (
        <div className="mt-3 overflow-x-auto rounded-xl border border-ink-100 bg-white shadow-card">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-ink-100 text-xs uppercase text-ink-500">
                <th className="px-4 py-3">Organisation</th>
                <th className="px-4 py-3">Status</th>
                <th className="px-4 py-3">Outlets</th>
                <th className="px-4 py-3">Contact</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-ink-100">
              {orgs.map((o) => (
                <tr key={o.id} className={cx("hover:bg-cream-50")}>
                  <td className="px-4 py-3 font-bold">{o.name}</td>
                  <td className="px-4 py-3">
                    <Badge tone={o.status === "approved" ? "leaf" : o.status === "suspended" || o.status === "rejected" ? "berry" : "marigold"}>
                      {o.status}
                    </Badge>
                  </td>
                  <td className="px-4 py-3">{o.outlets}</td>
                  <td className="px-4 py-3 text-ink-500">{o.contact_email}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {reviewing ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4" role="dialog" aria-modal="true">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">Review {reviewing.organisation}</h2>
            <p className="mt-1 text-xs text-ink-500">
              Demo approval is a simulation. Real deployments require authorised human verification.
            </p>
            <div className="mt-3 flex gap-1">
              {(["approve", "changes_requested", "reject"] as const).map((d) => (
                <button
                  key={d}
                  onClick={() => setDecision(d)}
                  aria-pressed={decision === d}
                  className={cx(
                    "rounded-full px-3 py-1.5 text-xs font-bold",
                    decision === d ? "bg-ink-900 text-cream-50" : "bg-cream-100 text-ink-600",
                  )}
                >
                  {d.replace(/_/g, " ")}
                </button>
              ))}
            </div>
            <textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              rows={3}
              aria-label="Review note"
              placeholder="Review note (required, audited)…"
              className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
            />
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setReviewing(null)}>Back</Button>
              <Button onClick={review} disabled={note.trim().length < 4} loading={busy}>Submit review</Button>
            </div>
          </div>
        </div>
      ) : null}
    </AdminShell>
  );
}

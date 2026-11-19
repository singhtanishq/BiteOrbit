"use client";

import { useCallback, useEffect, useState } from "react";
import { adminAuditLogs, AuditRow } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { EmptyState, Skeleton } from "@biteorbit/ui";

export default function AuditPage() {
  const { ready } = useAdminScope();
  const [rows, setRows] = useState<AuditRow[] | null>(null);
  const [error, setError] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setRows(await adminAuditLogs());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  return (
    <AdminShell
      active="audit"
      title="Audit log"
      subtitle="Every administrative action: actor, target, before/after and reason. Rows are immutable."
    >
      {error ? <p role="alert" className="rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}
      {!rows ? (
        <Skeleton className="mt-4 h-64 w-full" />
      ) : rows.length === 0 ? (
        <EmptyState icon="📜" title="No audit entries yet" description="Interventions, reviews and flag changes appear here." className="mt-4" />
      ) : (
        <ul className="mt-4 space-y-2">
          {rows.map((l) => (
            <li key={l.id} className="rounded-xl border border-ink-100 bg-white px-5 py-3 shadow-card">
              <div className="flex flex-wrap items-center justify-between gap-2">
                <span className="font-bold">{l.action.replace(/\./g, " · ")}</span>
                <span className="text-xs text-ink-500">{new Date(l.created_at).toLocaleString("en-IN")}</span>
              </div>
              <p className="mt-1 text-xs text-ink-600">
                {l.actor_type} → {l.target_type}:{l.target_id}
              </p>
              {l.reason ? <p className="mt-1 text-xs italic text-ink-500">“{l.reason}”</p> : null}
              <details className="mt-1 text-[11px] text-ink-400">
                <summary className="cursor-pointer">state diff</summary>
                <pre className="mt-1 overflow-x-auto whitespace-pre-wrap">{l.before_state}
→ {l.after_state}</pre>
              </details>
            </li>
          ))}
        </ul>
      )}
    </AdminShell>
  );
}

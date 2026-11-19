"use client";

import { useCallback, useEffect, useState } from "react";
import { adminFlags, adminUpdateFlag, AdminFlag } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Skeleton, cx } from "@biteorbit/ui";

export default function FlagsPage() {
  const { ready } = useAdminScope();
  const [flags, setFlags] = useState<AdminFlag[] | null>(null);
  const [error, setError] = useState("");
  const [busyKey, setBusyKey] = useState("");

  const load = useCallback(async () => {
    setError("");
    try {
      setFlags(await adminFlags());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const toggle = async (f: AdminFlag) => {
    setBusyKey(f.key);
    try {
      await adminUpdateFlag(f.key, !f.enabled, "Toggled from admin console (demo)");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setBusyKey("");
    }
  };

  return (
    <AdminShell active="flags" title="Feature flags" subtitle="Toggles are audited. Flags gate features without redeploying.">
      {error ? <p role="alert" className="rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}
      {!flags ? (
        <Skeleton className="mt-4 h-40 w-full" />
      ) : (
        <ul className="mt-4 space-y-2">
          {flags.map((f) => (
            <li key={f.key} className="flex items-center justify-between rounded-xl border border-ink-100 bg-white px-5 py-3.5 shadow-card">
              <div>
                <p className="font-bold">{f.key}</p>
                <p className="text-xs text-ink-500">{f.description}</p>
              </div>
              <button
                role="switch"
                aria-checked={f.enabled}
                onClick={() => toggle(f)}
                disabled={busyKey === f.key}
                className={cx(
                  "relative h-6 w-11 rounded-full transition-colors",
                  f.enabled ? "bg-leaf-600" : "bg-ink-300",
                  busyKey === f.key && "opacity-50",
                )}
                aria-label={`Toggle ${f.key}`}
              >
                <span
                  className={cx(
                    "absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all",
                    f.enabled ? "left-[22px]" : "left-0.5",
                  )}
                />
              </button>
            </li>
          ))}
        </ul>
      )}
    </AdminShell>
  );
}

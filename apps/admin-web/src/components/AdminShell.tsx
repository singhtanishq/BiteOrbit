"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { friendlyError, getSession, login, logout } from "@biteorbit/api-client";
import { Button, Logo, cx } from "@biteorbit/ui";

const NAV = [
  ["overview", "/overview", "Overview"],
  ["orders", "/orders", "Orders"],
  ["restaurants", "/restaurants", "Restaurants"],
  ["riders", "/riders", "Riders"],
  ["coupons", "/coupons", "Coupons"],
  ["support", "/support", "Support"],
  ["audit", "/audit", "Audit log"],
  ["flags", "/flags", "Flags"],
] as const;

/** Admin console shell with role guard (admin/ops/support). */
export function useAdminScope() {
  const [user, setUser] = useState<{ name: string; email: string; roles: string[] } | null>(null);
  const [ready, setReady] = useState(false);
  const router = useRouter();

  useEffect(() => {
    (async () => {
      try {
        const u = await getSession();
        const ok = u.roles.some((r) => r === "admin" || r === "ops_manager" || r === "support_agent");
        if (!ok) {
          router.replace("/login");
          return;
        }
        setUser({ name: u.name, email: u.email, roles: u.roles });
        setReady(true);
      } catch {
        router.replace("/login");
      }
    })();
  }, [router]);

  return { user, ready };
}

export function AdminShell({
  children,
  active,
  title,
  subtitle,
}: {
  children: React.ReactNode;
  active: string;
  title: string;
  subtitle?: string;
}) {
  const { user, ready } = useAdminScope();
  const router = useRouter();
  const signOut = useCallback(async () => {
    await logout();
    router.push("/login");
  }, [router]);

  if (!ready) {
    return <main className="mx-auto max-w-7xl px-4 py-10 text-sm text-ink-500">Loading console…</main>;
  }

  return (
    <div className="mx-auto max-w-7xl px-4 py-6">
      <header className="flex flex-wrap items-center justify-between gap-3 border-b border-ink-100 pb-4">
        <div className="flex items-center gap-4">
          <Logo />
          <BadgePill roles={user?.roles ?? []} />
        </div>
        <Button size="sm" variant="ghost" onClick={signOut}>
          Sign out ({user?.email})
        </Button>
      </header>
      <nav aria-label="Admin sections" className="mt-4 flex flex-wrap gap-1 text-sm font-semibold">
        {NAV.map(([key, href, label]) => (
          <a
            key={key}
            href={href}
            aria-current={active === key ? "page" : undefined}
            className={cx(
              "rounded-full px-3 py-1.5",
              active === key ? "bg-ink-900 text-cream-50" : "text-ink-700 hover:bg-cream-200",
            )}
          >
            {label}
          </a>
        ))}
      </nav>
      <main className="mt-6">
        <h1 className="text-2xl font-extrabold">{title}</h1>
        {subtitle ? <p className="mt-1 text-sm text-ink-500">{subtitle}</p> : null}
        <div className="mt-5">{children}</div>
      </main>
    </div>
  );
}

function BadgePill({ roles }: { roles: string[] }) {
  const top = roles.includes("admin") ? "admin" : roles.includes("ops_manager") ? "ops" : "support";
  return (
    <span className="rounded-full bg-cream-200 px-3 py-1 text-xs font-bold text-ink-700">
      {top} console
    </span>
  );
}

export function AdminLogin({ onDone }: { onDone?: () => void }) {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const u = await login(email, password);
      const ok = u.roles.some((r) => r === "admin" || r === "ops_manager" || r === "support_agent");
      if (!ok) {
        setError("This console is for platform staff.");
        return;
      }
      if (onDone) {
        onDone();
      }
      router.push("/overview");
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <main className="mx-auto max-w-md px-4 py-16">
      <div className="mb-6 flex justify-center">
        <Logo />
      </div>
      <div className="rounded-xl border border-ink-100 bg-white p-6 shadow-card">
        <h1 className="text-lg font-extrabold">Platform staff sign-in</h1>
        <form onSubmit={submit} className="mt-4 space-y-3" noValidate>
          <input
            type="email" value={email} onChange={(e) => setEmail(e.target.value)}
            placeholder="Email" aria-label="Email" required
            className="h-11 w-full rounded-lg border border-ink-200 px-3 text-sm"
          />
          <input
            type="password" value={password} onChange={(e) => setPassword(e.target.value)}
            placeholder="Password" aria-label="Password" required
            className="h-11 w-full rounded-lg border border-ink-200 px-3 text-sm"
          />
          {error ? <p role="alert" className="text-sm font-medium text-berry-600">{error}</p> : null}
          <Button className="w-full" type="submit" loading={busy}>Sign in</Button>
        </form>
        <div className="mt-4 rounded-lg bg-cream-100 px-4 py-3 text-xs text-ink-600">
          <p className="font-bold">Demo staff (local only)</p>
          <p className="mt-1"><code>admin@demo.biteorbit.local</code> / <code>Demo@1234</code></p>
          <p className="mt-1 text-[11px]">ops@… (ops_manager), support@… (support_agent)</p>
          <button
            className="mt-2 rounded-md bg-white px-2 py-1 font-semibold text-coral-600"
            onClick={() => { setEmail("admin@demo.biteorbit.local"); setPassword("Demo@1234"); }}
          >
            Fill admin credentials
          </button>
        </div>
      </div>
    </main>
  );
}

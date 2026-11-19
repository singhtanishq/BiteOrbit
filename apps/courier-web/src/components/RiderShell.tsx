"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { friendlyError, getSession, login, logout } from "@biteorbit/api-client";
import { Button, Logo, cx } from "@biteorbit/ui";
import { riderProfile, setAvailability, RiderProfile } from "@/lib/courier";

/** Shared rider shell: auth guard, availability toggle, nav. */
export function useRiderScope() {
  const [profile, setProfile] = useState<RiderProfile | null>(null);
  const [ready, setReady] = useState(false);
  const router = useRouter();

  const refresh = useCallback(async () => {
    try {
      const p = await riderProfile();
      setProfile(p);
      setReady(true);
    } catch (err) {
      const msg = friendlyError(err);
      if (msg.includes("sign in") || msg.includes("Please")) {
        router.replace("/login");
      }
      setReady(true);
    }
  }, [router]);

  useEffect(() => {
    (async () => {
      try {
        const u = await getSession();
        if (!u.roles.some((r) => r === "rider")) {
          router.replace("/login");
          return;
        }
      } catch {
        router.replace("/login");
        return;
      }
      await refresh();
    })();
  }, [refresh, router]);

  const toggle = useCallback(
    async (online: boolean) => {
      const p = await setAvailability(online);
      setProfile((prev) => (prev ? { ...prev, availability: p.availability } : prev));
    },
    [],
  );

  return { profile, ready, toggle, refresh };
}

export function RiderShell({
  children,
  profile,
  onToggle,
  active,
}: {
  children: React.ReactNode;
  profile: RiderProfile | null;
  onToggle: (online: boolean) => void;
  active: "today" | "offers" | "earnings";
}) {
  const router = useRouter();
  const online = profile?.availability === "online";

  return (
    <div className="mx-auto max-w-md px-4 py-5">
      <div className="flex items-center justify-between">
        <Logo />
        <div className="flex items-center gap-2">
          {profile ? (
            <button
              role="switch"
              aria-checked={online}
              onClick={() => onToggle(!online)}
              disabled={profile.status !== "approved"}
              className={cx(
                "flex items-center gap-2 rounded-full px-3 py-1.5 text-xs font-bold transition-colors",
                online ? "bg-leaf-600 text-white" : "bg-cream-200 text-ink-600",
                profile.status !== "approved" && "opacity-50",
              )}
            >
              <span
                className={cx(
                  "inline-block h-2.5 w-2.5 rounded-full",
                  online ? "bg-white animate-pulse" : "bg-ink-400",
                )}
              />
              {online ? "ONLINE" : "OFFLINE"}
            </button>
          ) : null}
          <Button
            size="sm"
            variant="ghost"
            onClick={async () => {
              await logout();
              router.push("/login");
            }}
          >
            Sign out
          </Button>
        </div>
      </div>
      {profile && profile.status !== "approved" ? (
        <p className="mt-4 rounded-lg bg-marigold-50 px-4 py-3 text-sm text-ink-700">
          Your account is <strong>{profile.status}</strong>. Activation is part of the demo
          review simulation — go online once approved.
        </p>
      ) : null}
      <nav aria-label="Rider" className="mt-4 flex gap-1 text-sm font-semibold">
        {(
          [
            ["today", "/dashboard", "Today"],
            ["offers", "/offers", "Offers"],
            ["earnings", "/earnings", "Earnings"],
          ] as const
        ).map(([key, href, label]) => (
          <a
            key={key}
            href={href}
            aria-current={active === key ? "page" : undefined}
            className={cx(
              "flex-1 rounded-lg px-3 py-2 text-center",
              active === key ? "bg-coral-500 text-white" : "bg-cream-100 text-ink-700 hover:bg-cream-200",
            )}
          >
            {label}
          </a>
        ))}
      </nav>
      {children}
      <p className="mt-8 text-center text-[11px] text-ink-400">
        Local demo — dispatch, positions and payouts are simulated.
      </p>
    </div>
  );
}

export function RiderLogin() {
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
      if (!u.roles.includes("rider")) {
        setError("This app is for delivery-partner accounts.");
        return;
      }
      router.push("/dashboard");
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
        <h1 className="text-lg font-extrabold">Delivery partner sign-in</h1>
        <form onSubmit={submit} className="mt-4 space-y-3" noValidate>
          <input
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Email"
            aria-label="Email"
            required
            className="h-11 w-full rounded-lg border border-ink-200 px-3 text-sm"
          />
          <input
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Password"
            aria-label="Password"
            required
            className="h-11 w-full rounded-lg border border-ink-200 px-3 text-sm"
          />
          {error ? (
            <p role="alert" className="text-sm font-medium text-berry-600">
              {error}
            </p>
          ) : null}
          <Button className="w-full" type="submit" loading={busy}>
            Sign in
          </Button>
        </form>
        <div className="mt-4 rounded-lg bg-cream-100 px-4 py-3 text-xs text-ink-600">
          <p className="font-bold">Demo rider (local only)</p>
          <p className="mt-1">
            <code>rider@demo.biteorbit.local</code> / <code>Demo@1234</code>
          </p>
          <button
            className="mt-2 rounded-md bg-white px-2 py-1 font-semibold text-coral-600"
            onClick={() => {
              setEmail("rider@demo.biteorbit.local");
              setPassword("Demo@1234");
            }}
          >
            Fill demo credentials
          </button>
        </div>
      </div>
    </main>
  );
}


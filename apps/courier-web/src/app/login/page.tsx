"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { friendlyError, login } from "@biteorbit/api-client";
import { Button, Logo } from "@biteorbit/ui";

export default function RiderLoginPage() {
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

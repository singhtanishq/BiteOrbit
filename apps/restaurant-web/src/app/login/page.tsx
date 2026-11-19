"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { friendlyError, login, SessionUser } from "@biteorbit/api-client";
import { Button, Card, CardBody, CardHeader, CardTitle, Input } from "@biteorbit/ui";

export default function PortalLoginPage() {
  const router = useRouter();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const user = await login(email, password);
      const isStaff = user.roles.some((r) => r.startsWith("restaurant_"));
      if (!isStaff) {
        setError("This console is for restaurant staff accounts.");
        return;
      }
      router.push("/dashboard");
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const fillDemo = () => {
    setEmail("manager01@demo.biteorbit.local");
    setPassword("Demo@1234");
  };

  return (
    <main className="mx-auto max-w-md px-4 py-16">
      <Card>
        <CardHeader>
          <CardTitle>Partner sign-in</CardTitle>
        </CardHeader>
        <CardBody>
          <form onSubmit={submit} className="space-y-4" noValidate>
            <Input label="Email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />
            <Input label="Password" type="password" value={password} onChange={(e) => setPassword(e.target.value)} required />
            {error ? (
              <p role="alert" className="text-sm font-medium text-berry-600">
                {error}
              </p>
            ) : null}
            <Button type="submit" className="w-full" loading={busy}>
              Sign in
            </Button>
          </form>
          <div className="mt-5 rounded-lg bg-cream-100 px-4 py-3 text-xs text-ink-600">
            <p className="font-bold text-ink-800">Demo partner (local only)</p>
            <p className="mt-1">
              <code className="rounded bg-white px-1">manager01@demo.biteorbit.local</code> /{" "}
              <code className="rounded bg-white px-1">Demo@1234</code>
            </p>
            <Button size="sm" variant="secondary" className="mt-2" onClick={fillDemo}>
              Fill demo credentials
            </Button>
          </div>
          <p className="mt-3 text-[11px] text-ink-400">
            Sessions are HttpOnly cookies; roles are enforced server-side on every call.
          </p>
        </CardBody>
      </Card>
    </main>
  );
}

export type { SessionUser };

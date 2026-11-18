"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Address,
  createAddress,
  friendlyError,
  getSession,
  listAddresses,
  login,
  logout,
  register,
  SessionUser,
  ApiError,
} from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, EmptyState, Input, Select } from "@biteorbit/ui";

export default function AccountPage() {
  const [user, setUser] = useState<SessionUser | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    getSession()
      .then(setUser)
      .catch(() => setUser(null))
      .finally(() => setLoaded(true));
  }, []);

  if (!loaded) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-10">
        <div className="h-40 animate-pulse rounded-xl bg-ink-100/60" aria-hidden="true" />
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Account</h1>
      <p className="mt-1 text-sm text-ink-500">
        Local demo accounts only — no real personal data should be entered here.
      </p>
      {user ? (
        <SignedInPanel user={user} onSignedOut={() => setUser(null)} />
      ) : (
        <AuthPanel onSignedIn={setUser} />
      )}
    </main>
  );
}

function AuthPanel({ onSignedIn }: { onSignedIn: (u: SessionUser) => void }) {
  const [mode, setMode] = useState<"signin" | "signup">("signin");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [name, setName] = useState("");
  const [phone, setPhone] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      const user =
        mode === "signin"
          ? await login(email, password)
          : await register(email, password, name, phone || undefined);
      onSignedIn(user);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const fillDemo = () => {
    setMode("signin");
    setEmail("customer@demo.biteorbit.local");
    setPassword("Demo@1234");
  };

  return (
    <Card className="mt-6">
      <CardHeader>
        <CardTitle>{mode === "signin" ? "Sign in" : "Create your account"}</CardTitle>
      </CardHeader>
      <CardBody>
        <form onSubmit={submit} className="space-y-4" noValidate>
          {mode === "signup" ? (
            <>
              <Input
                label="Full name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
                autoComplete="name"
              />
              <Input
                label="Phone (optional)"
                value={phone}
                onChange={(e) => setPhone(e.target.value)}
                inputMode="tel"
                hint="10–14 digits, optionally with +"
              />
            </>
          ) : null}
          <Input
            label="Email"
            type="email"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            autoComplete="email"
          />
          <Input
            label="Password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoComplete={mode === "signin" ? "current-password" : "new-password"}
            hint={mode === "signup" ? "At least 10 chars with upper, lower and a digit" : undefined}
          />
          {error ? (
            <p role="alert" className="text-sm font-medium text-berry-600">
              {error}
            </p>
          ) : null}
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" loading={busy}>
              {mode === "signin" ? "Sign in" : "Create account"}
            </Button>
            <Button
              type="button"
              variant="ghost"
              onClick={() => setMode(mode === "signin" ? "signup" : "signin")}
            >
              {mode === "signin" ? "Need an account? Register" : "Have an account? Sign in"}
            </Button>
          </div>
        </form>
        <div className="mt-6 rounded-lg bg-cream-100 px-4 py-3 text-xs text-ink-600">
          <p className="font-bold text-ink-800">Demo customer shortcut</p>
          <p className="mt-1">
            Use the seeded demo customer (synthetic data, local only):{" "}
            <code className="rounded bg-white px-1">customer@demo.biteorbit.local</code> /{" "}
            <code className="rounded bg-white px-1">Demo@1234</code>
          </p>
          <Button size="sm" variant="secondary" className="mt-2" onClick={fillDemo}>
            Fill demo credentials
          </Button>
        </div>
      </CardBody>
    </Card>
  );
}

function SignedInPanel({ user, onSignedOut }: { user: SessionUser; onSignedOut: () => void }) {
  const [addresses, setAddresses] = useState<Address[] | null>(null);
  const [showAdd, setShowAdd] = useState(false);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      setAddresses(await listAddresses());
    } catch (err) {
      setError(friendlyError(err));
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const signOut = async () => {
    try {
      await logout();
    } finally {
      onSignedOut();
    }
  };

  return (
    <div className="mt-6 space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>Profile</CardTitle>
        </CardHeader>
        <CardBody className="text-sm text-ink-700">
          <p>
            <strong>{user.name}</strong> <span className="text-ink-500">({user.email})</span>
          </p>
          <div className="mt-2 flex gap-2">
            {user.roles.map((r) => (
              <Badge key={r} tone="sky">
                {r}
              </Badge>
            ))}
          </div>
          <Button variant="secondary" size="sm" className="mt-4" onClick={signOut}>
            Sign out
          </Button>
        </CardBody>
      </Card>

      <Card>
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>Saved addresses</CardTitle>
            <Button size="sm" onClick={() => setShowAdd((v) => !v)}>
              {showAdd ? "Cancel" : "Add address"}
            </Button>
          </div>
        </CardHeader>
        <CardBody>
          {error ? <p role="alert" className="text-sm text-berry-600">{error}</p> : null}
          {addresses === null ? (
            <div className="h-20 animate-pulse rounded-lg bg-ink-100/60" aria-hidden="true" />
          ) : addresses.length === 0 && !showAdd ? (
            <EmptyState
              icon="🏠"
              title="No saved addresses"
              description="Add one to check delivery serviceability in the demo city."
              action={<Button size="sm" onClick={() => setShowAdd(true)}>Add address</Button>}
            />
          ) : null}
          {addresses && addresses.length > 0 ? (
            <ul className="space-y-3">
              {addresses.map((a) => (
                <li key={a.id} className="rounded-lg border border-ink-100 px-4 py-3 text-sm">
                  <div className="flex items-center justify-between">
                    <span className="font-bold capitalize text-ink-900">{a.label}</span>
                    {a.is_default ? <Badge tone="leaf">Default</Badge> : null}
                  </div>
                  <p className="mt-1 text-ink-600">
                    {a.line1}
                    {a.line2 ? `, ${a.line2}` : ""}, {a.locality}, {a.city} {a.postal_code}
                  </p>
                  <p className="text-xs text-ink-500">
                    {a.recipient_name} · {a.recipient_phone}
                    {a.delivery_instructions ? ` · ${a.delivery_instructions}` : ""}
                  </p>
                </li>
              ))}
            </ul>
          ) : null}
          {showAdd ? (
            <AddressForm
              onSaved={() => {
                setShowAdd(false);
                refresh();
              }}
            />
          ) : null}
        </CardBody>
      </Card>
    </div>
  );
}

function AddressForm({ onSaved }: { onSaved: () => void }) {
  const [form, setForm] = useState({
    label: "home",
    recipient_name: "",
    recipient_phone: "",
    line1: "",
    line2: "",
    landmark: "",
    locality: "",
    city: "Bitepur",
    state: "Telengana Heights",
    postal_code: "",
    delivery_instructions: "",
  });
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError("");
    setBusy(true);
    try {
      await createAddress({
        label: form.label as "home" | "work" | "other",
        recipient_name: form.recipient_name,
        recipient_phone: form.recipient_phone,
        line1: form.line1,
        line2: form.line2 || null,
        landmark: form.landmark || null,
        locality: form.locality,
        city: form.city,
        state: form.state,
        postal_code: form.postal_code,
        delivery_instructions: form.delivery_instructions || null,
        is_default: false,
      });
      onSaved();
    } catch (err) {
      if (err instanceof ApiError && err.code === "validation_failed") setError(err.message);
      else setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <form onSubmit={submit} className="mt-4 space-y-3 rounded-lg bg-cream-100 p-4" noValidate>
      <div className="grid gap-3 sm:grid-cols-2">
        <Select label="Label" value={form.label} onChange={set("label")}>
          <option value="home">Home</option>
          <option value="work">Work</option>
          <option value="other">Other</option>
        </Select>
        <Input label="Recipient name" value={form.recipient_name} onChange={set("recipient_name")} required />
        <Input label="Phone" value={form.recipient_phone} onChange={set("recipient_phone")} required inputMode="tel" />
        <Input label="Pincode" value={form.postal_code} onChange={set("postal_code")} required inputMode="numeric" hint="6-digit Indian pincode" />
        <Input label="Address line 1" value={form.line1} onChange={set("line1")} required className="sm:col-span-2" />
        <Input label="Line 2 (optional)" value={form.line2} onChange={set("line2")} />
        <Input label="Landmark (optional)" value={form.landmark} onChange={set("landmark")} />
        <Input label="Locality" value={form.locality} onChange={set("locality")} required hint='Try "Rosewood Hills" or another demo locality' />
        <Input label="City" value={form.city} onChange={set("city")} required />
        <Input label="State" value={form.state} onChange={set("state")} required />
        <Input
          label="Delivery instructions (optional)"
          value={form.delivery_instructions}
          onChange={set("delivery_instructions")}
          className="sm:col-span-2"
        />
      </div>
      {error ? (
        <p role="alert" className="text-sm font-medium text-berry-600">
          {error}
        </p>
      ) : null}
      <Button type="submit" loading={busy}>
        Save address
      </Button>
    </form>
  );
}

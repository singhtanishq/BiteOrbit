"use client";

import { useCallback, useEffect, useState } from "react";
import { listPortalMenu, updatePortalMenuItem, PortalMenuItem } from "@/lib/portal";
import { PortalShell, useOutletScope } from "@/components/PortalNav";
import { Badge, Button, EmptyState, Input, Skeleton, cx, formatINR } from "@biteorbit/ui";

type EditState = { id: string; price: string; name: string } | null;

export default function MenuManagementPage() {
  const { outlets, outlet, outletId, setOutletId, ready } = useOutletScope();
  const [categories, setCategories] = useState<Array<{ id: string; name: string; items: PortalMenuItem[] }> | null>(null);
  const [error, setError] = useState("");
  const [busyId, setBusyId] = useState("");
  const [editing, setEditing] = useState<EditState>(null);
  const [notice, setNotice] = useState("");

  const load = useCallback(async () => {
    if (!outletId) return;
    setError("");
    try {
      const menu = await listPortalMenu(outletId);
      setCategories(menu.categories);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load menu");
    }
  }, [outletId]);

  useEffect(() => {
    load();
  }, [load]);

  const patch = async (id: string, body: Parameters<typeof updatePortalMenuItem>[1]) => {
    setBusyId(id);
    setError("");
    setNotice("");
    try {
      await updatePortalMenuItem(id, body);
      await load();
      setNotice("Menu updated — changes are audited and reflected to customers immediately.");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setBusyId("");
    }
  };

  const saveEdit = async () => {
    if (!editing) return;
    const price = Math.round(parseFloat(editing.price) * 100);
    if (!Number.isFinite(price) || price < 100) {
      setError("Price must be at least ₹1.");
      return;
    }
    await patch(editing.id, { price_paise: price, name: editing.name.trim() || undefined });
    setEditing(null);
  };

  if (!ready) {
    return <main className="mx-auto max-w-6xl px-4 py-10"><Skeleton className="h-40 w-full" /></main>;
  }

  return (
    <PortalShell outletId={outletId} outlets={outlets ?? []} onOutletChange={setOutletId} active="menu">
      <main className="mt-6">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h1 className="text-2xl font-extrabold">Menu management</h1>
            <p className="text-xs text-ink-500">{outlet?.name} · {outlet?.locality}</p>
          </div>
          <Button size="sm" variant="secondary" onClick={load}>Refresh</Button>
        </div>

        {notice ? (
          <p role="status" className="mt-4 rounded-lg bg-leaf-50 px-4 py-3 text-sm text-leaf-700">
            {notice}
          </p>
        ) : null}
        {error ? (
          <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">
            {error}
          </p>
        ) : null}

        {!categories ? (
          <div className="mt-6 space-y-3">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-24 w-full" />
            ))}
          </div>
        ) : categories.length === 0 ? (
          <EmptyState icon="🍳" title="No menu yet" description="Menu creation arrives with the admin milestone." className="mt-6" />
        ) : (
          categories.map((c) => (
            <section key={c.id} className="mt-8" aria-label={c.name}>
              <h2 className="text-lg font-extrabold">{c.name}</h2>
              <ul className="mt-3 divide-y divide-ink-100 overflow-hidden rounded-xl border border-ink-100 bg-white">
                {c.items.map((it) => (
                  <li key={it.id} className="flex flex-wrap items-center justify-between gap-3 px-5 py-4">
                    <div className="min-w-0 flex-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span
                          aria-label={it.is_veg ? "Vegetarian" : "Non-vegetarian"}
                          className={cx(
                            "inline-flex h-3.5 w-3.5 items-center justify-center rounded-[3px] border-2",
                            it.is_veg ? "border-leaf-600" : "border-berry-500",
                          )}
                        >
                          <span className={cx("h-1.5 w-1.5 rounded-full", it.is_veg ? "bg-leaf-600" : "bg-berry-500")} />
                        </span>
                        <span className="font-bold text-ink-900">{it.name}</span>
                        {it.is_bestseller ? <Badge tone="marigold">Bestseller</Badge> : null}
                        {!it.is_available ? <Badge tone="berry">Out of stock</Badge> : null}
                      </div>
                      <p className="mt-0.5 text-xs text-ink-500">
                        {it.description || "No description"}
                        {it.allergen_notes ? ` · contains: ${it.allergen_notes}` : ""}
                        {it.modifier_groups.length > 0 ? ` · ${it.modifier_groups.length} option group(s)` : ""}
                      </p>
                      {editing?.id === it.id ? (
                        <div className="mt-2 flex flex-wrap gap-2">
                          <Input
                            value={editing.name}
                            onChange={(e) => setEditing({ ...editing, name: e.target.value })}
                            aria-label="Item name"
                            className="w-48"
                          />
                          <Input
                            value={editing.price}
                            onChange={(e) => setEditing({ ...editing, price: e.target.value })}
                            aria-label="Price in rupees"
                            inputMode="decimal"
                            className="w-28"
                            hint="₹"
                          />
                          <Button size="sm" onClick={saveEdit} loading={busyId === it.id}>
                            Save
                          </Button>
                          <Button size="sm" variant="ghost" onClick={() => setEditing(null)}>
                            Cancel
                          </Button>
                        </div>
                      ) : null}
                    </div>
                    <div className="flex shrink-0 items-center gap-3">
                      <span className="text-sm font-bold">{formatINR(it.price_paise)}</span>
                      {editing?.id !== it.id ? (
                        <Button
                          size="sm"
                          variant="secondary"
                          onClick={() => setEditing({ id: it.id, price: (it.price_paise / 100).toFixed(2), name: it.name })}
                        >
                          Edit
                        </Button>
                      ) : null}
                      <button
                        role="switch"
                        aria-checked={it.is_available}
                        onClick={() => patch(it.id, { is_available: !it.is_available })}
                        disabled={busyId === it.id}
                        className={cx(
                          "relative h-6 w-11 rounded-full transition-colors",
                          it.is_available ? "bg-leaf-600" : "bg-ink-300",
                          busyId === it.id && "opacity-50",
                        )}
                        aria-label={`${it.is_available ? "Mark unavailable" : "Mark available"}: ${it.name}`}
                      >
                        <span
                          className={cx(
                            "absolute top-0.5 h-5 w-5 rounded-full bg-white shadow transition-all",
                            it.is_available ? "left-[22px]" : "left-0.5",
                          )}
                        />
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            </section>
          ))
        )}
      </main>
    </PortalShell>
  );
}

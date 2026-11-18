"use client";

import { useMemo, useState } from "react";
import { apiRequest, friendlyError, MenuItem } from "@biteorbit/api-client";
import { Badge, Button, cx, formatINR } from "@biteorbit/ui";

export interface CartConflict {
  currentOutlet: string;
}

/**
 * Item customisation sheet: modifier selection (respecting required/min/max),
 * quantity, instructions and a live server-consistent total preview.
 * The server re-validates everything on add — this UI mirrors its rules.
 */
export function ItemSheet({
  item,
  onClose,
  onAdded,
}: {
  item: MenuItem;
  onClose: () => void;
  onAdded: () => void;
}) {
  const [selected, setSelected] = useState<Record<string, string[]>>(() => {
    const init: Record<string, string[]> = {};
    for (const g of item.modifier_groups) {
      const def = g.options.filter((o) => o.is_default && o.is_available).map((o) => o.id);
      init[g.id] = def.length > 0 ? def.slice(0, Math.max(g.min_select, 1)) : [];
    }
    return init;
  });
  const [qty, setQty] = useState(1);
  const [instructions, setInstructions] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [conflict, setConflict] = useState<CartConflict | null>(null);

  const unitTotal = useMemo(() => {
    let total = item.price_paise;
    for (const g of item.modifier_groups) {
      for (const oid of selected[g.id] ?? []) {
        const opt = g.options.find((o) => o.id === oid);
        if (opt) total += opt.price_delta_paise;
      }
    }
    return total;
  }, [item, selected]);

  const toggleOption = (groupId: string, optionId: string, max: number) => {
    setSelected((prev) => {
      const current = prev[groupId] ?? [];
      if (current.includes(optionId)) {
        return { ...prev, [groupId]: current.filter((id) => id !== optionId) };
      }
      if (max === 1) return { ...prev, [groupId]: [optionId] };
      if (current.length >= max) return prev; // cap reached; ignore silently
      return { ...prev, [groupId]: [...current, optionId] };
    });
  };

  const validationError = useMemo(() => {
    for (const g of item.modifier_groups) {
      const n = (selected[g.id] ?? []).length;
      if (g.is_required && n < g.min_select) {
        return `${g.name}: pick at least ${g.min_select}.`;
      }
    }
    return "";
  }, [item, selected]);

  const addToCart = async (replaceCart: boolean) => {
    setBusy(true);
    setError("");
    try {
      await apiRequest("/api/v1/cart/items", {
        method: "POST",
        body: {
          menu_item_id: item.id,
          quantity: qty,
          modifier_options: selected,
          special_instructions: instructions,
          replace_cart: replaceCart,
        },
      });
      window.dispatchEvent(new CustomEvent("cart-updated"));
      onAdded();
    } catch (err) {
      if (typeof err === "object" && err !== null && "code" in err && (err as { code?: string }).code === "cart_conflict") {
        const details = (err as { details?: Record<string, unknown> }).details;
        setConflict({
          currentOutlet: String(details?.["current_outlet"] ?? "another restaurant"),
        });
      } else {
        setError(friendlyError(err));
      }
    } finally {
      setBusy(false);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-end justify-center bg-ink-950/50 sm:items-center"
      role="dialog"
      aria-modal="true"
      aria-label={`Customise ${item.name}`}
      onClick={(e) => e.target === e.currentTarget && onClose()}
    >
      <div className="max-h-[92dvh] w-full max-w-lg overflow-y-auto rounded-t-2xl bg-white shadow-pop sm:rounded-2xl">
        <div className="sticky top-0 flex items-center justify-between border-b border-ink-100 bg-white px-5 py-4">
          <div className="flex items-center gap-2">
            <span
              aria-hidden="true"
              className={cx(
                "inline-flex h-4 w-4 items-center justify-center rounded-[4px] border-2",
                item.is_veg ? "border-leaf-600" : "border-berry-500",
              )}
            >
              <span className={cx("h-2 w-2 rounded-full", item.is_veg ? "bg-leaf-600" : "bg-berry-500")} />
            </span>
            <h2 className="text-lg font-extrabold text-ink-900">{item.name}</h2>
          </div>
          <button
            onClick={onClose}
            aria-label="Close"
            className="rounded-full p-1.5 text-ink-500 hover:bg-cream-200"
          >
            ✕
          </button>
        </div>

        <div className="px-5 py-4">
          <p className="text-sm text-ink-600">{item.description}</p>
          <p className="mt-1 text-base font-bold">{formatINR(item.price_paise)}</p>
          {item.serving_info ? <p className="mt-0.5 text-xs text-ink-400">{item.serving_info}</p> : null}
          {item.allergen_notes ? (
            <p className="mt-1 text-xs text-ink-500">
              <span className="font-semibold">Contains:</span> {item.allergen_notes}{" "}
              <span className="text-ink-400">(as entered by the restaurant)</span>
            </p>
          ) : null}

          {/* modifier groups */}
          {item.modifier_groups.map((g) => (
            <fieldset key={g.id} className="mt-5">
              <legend className="text-sm font-bold text-ink-900">
                {g.name}{" "}
                {g.is_required ? (
                  <span className="ml-1 text-xs font-medium text-berry-600">required</span>
                ) : (
                  <span className="ml-1 text-xs font-medium text-ink-500">optional</span>
                )}
                <span className="ml-2 text-xs font-medium text-ink-400">
                  pick {g.min_select}
                  {g.max_select > 1 ? `–${g.max_select}` : ""}
                </span>
              </legend>
              <div className="mt-2 space-y-2">
                {g.options.map((o) => {
                  const checked = (selected[g.id] ?? []).includes(o.id);
                  const disabled = !o.is_available || !item.is_available;
                  const inputType = g.max_select === 1 ? "radio" : "checkbox";
                  return (
                    <label
                      key={o.id}
                      className={cx(
                        "flex cursor-pointer items-center justify-between rounded-lg border px-3 py-2.5 text-sm transition-colors",
                        checked ? "border-coral-500 bg-coral-50" : "border-ink-200 bg-white",
                        disabled && "opacity-50",
                      )}
                    >
                      <span className="flex items-center gap-2.5">
                        <input
                          type={inputType}
                          name={g.id}
                          checked={checked}
                          disabled={disabled}
                          onChange={() => toggleOption(g.id, o.id, g.max_select)}
                          className={cx("accent-[color:var(--color-coral-500)]", inputType === "radio" ? "h-4 w-4" : "h-4 w-4")}
                        />
                        <span className="font-medium text-ink-800">{o.name}</span>
                        {!o.is_available ? <Badge tone="berry">out</Badge> : null}
                      </span>
                      <span className="text-ink-600">
                        {o.price_delta_paise > 0
                          ? `+${formatINR(o.price_delta_paise, { withDecimals: false })}`
                          : "Free"}
                      </span>
                    </label>
                  );
                })}
              </div>
            </fieldset>
          ))}

          {/* instructions */}
          <div className="mt-5">
            <label htmlFor="item-instructions" className="text-sm font-bold text-ink-900">
              Special instructions
            </label>
            <textarea
              id="item-instructions"
              value={instructions}
              maxLength={240}
              onChange={(e) => setInstructions(e.target.value)}
              placeholder="e.g., less spicy, no onion"
              className="mt-1.5 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm focus:border-coral-500 focus:outline-2 focus:outline-coral-500"
            />
            <p className="mt-1 text-right text-xs text-ink-400">{instructions.length}/240</p>
          </div>
        </div>

        {/* footer with qty + total + add */}
        <div className="sticky bottom-0 border-t border-ink-100 bg-white px-5 py-4">
          {error ? (
            <p role="alert" className="mb-2 text-sm font-medium text-berry-600">
              {error}
            </p>
          ) : null}
          {conflict ? (
            <div className="mb-3 rounded-lg bg-marigold-50 px-4 py-3 text-sm text-ink-800">
              <p className="font-bold">
                Replace your cart from {conflict.currentOutlet}?
              </p>
              <p className="mt-1 text-xs text-ink-600">
                Adding from {item.name}&apos;s restaurant removes your existing cart items. We
                never do this silently.
              </p>
              <div className="mt-2 flex gap-2">
                <Button size="sm" variant="danger" onClick={() => addToCart(true)} loading={busy}>
                  Replace cart
                </Button>
                <Button size="sm" variant="secondary" onClick={() => setConflict(null)}>
                  Keep current cart
                </Button>
              </div>
            </div>
          ) : null}
          <div className="flex items-center gap-3">
            <div className="flex items-center rounded-lg border border-ink-200">
              <button
                onClick={() => setQty((q) => Math.max(1, q - 1))}
                aria-label="Decrease quantity"
                className="px-3 py-2 text-lg font-bold text-ink-600 hover:text-coral-600"
              >
                −
              </button>
              <span aria-live="polite" className="w-8 text-center text-sm font-bold">
                {qty}
              </span>
              <button
                onClick={() => setQty((q) => Math.min(20, q + 1))}
                aria-label="Increase quantity"
                className="px-3 py-2 text-lg font-bold text-ink-600 hover:text-coral-600"
              >
                +
              </button>
            </div>
            <Button
              className="flex-1"
              size="lg"
              loading={busy}
              disabled={Boolean(validationError) || !item.is_available}
              onClick={() => addToCart(false)}
            >
              {item.is_available ? `Add · ${formatINR(unitTotal * qty)}` : "Unavailable"}
            </Button>
          </div>
          {validationError ? (
            <p className="mt-2 text-xs font-medium text-berry-600">{validationError}</p>
          ) : null}
        </div>
      </div>
    </div>
  );
}

"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { apiRequest, friendlyError } from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, EmptyState, Skeleton, cx, formatINR } from "@biteorbit/ui";

interface CartItem {
  id: string;
  name: string;
  is_veg: boolean;
  quantity: number;
  unit_price_paise: number;
  line_total_paise: number;
  special_instructions: string | null;
  modifiers: Array<{ option_id: string; group_name: string; option_name: string; price_delta_paise: number }>;
  modifiers_total_paise: number;
  availability_state: "ok" | "unavailable" | "price_changed" | "missing";
  current_price_paise: number;
  current_available: boolean;
}

interface CartData {
  id: string;
  outlet_id: string;
  outlet_name: string;
  outlet_locality: string;
  items: CartItem[];
  items_subtotal_paise: number;
}

/**
 * Cart page: line management with server-authoritative prices.
 * The server recomputes every amount on every response — the UI only renders.
 */
export default function CartPage() {
  const router = useRouter();
  const [cart, setCart] = useState<CartData | null>(null);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const refresh = useCallback(async () => {
    setError("");
    try {
      const data = await apiRequest<CartData | null>("/api/v1/cart");
      setCart(data);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setLoaded(true);
    }
  }, []);

  useEffect(() => {
    refresh();
  }, [refresh]);

  const updateQty = async (itemId: string, quantity: number) => {
    setBusy(true);
    try {
      const data = await apiRequest<CartData | null>(`/api/v1/cart/items/${itemId}`, {
        method: "PATCH",
        body: { quantity },
      });
      setCart(data);
      window.dispatchEvent(new CustomEvent("cart-updated"));
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const removeItem = async (itemId: string) => {
    setBusy(true);
    try {
      const data = await apiRequest<CartData | null>(`/api/v1/cart/items/${itemId}`, { method: "DELETE" });
      setCart(data);
      window.dispatchEvent(new CustomEvent("cart-updated"));
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const clearCart = async () => {
    setBusy(true);
    try {
      await apiRequest("/api/v1/cart", { method: "DELETE" });
      setCart(null);
      window.dispatchEvent(new CustomEvent("cart-updated"));
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  if (!loaded) {
    return (
      <main className="mx-auto max-w-3xl px-4 py-10">
        <h1 className="text-2xl font-extrabold">Your cart</h1>
        <Skeleton className="mt-6 h-40 w-full" />
      </main>
    );
  }

  const hasProblems = cart?.items.some((it) => it.availability_state !== "ok") ?? false;

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-extrabold">Your cart</h1>
        {cart && cart.items.length > 0 ? (
          <Button variant="ghost" size="sm" onClick={clearCart} disabled={busy}>
            Clear cart
          </Button>
        ) : null}
      </div>

      {error ? (
        <p role="alert" className="mt-4 text-sm font-medium text-berry-600">
          {error}
        </p>
      ) : null}

      {!cart || cart.items.length === 0 ? (
        <EmptyState
          icon="🛒"
          title="Your cart is empty"
          description="Add dishes from Bitepur's restaurants — one restaurant per order."
          className="mt-6"
          action={
            <Link href="/">
              <Button>Browse restaurants</Button>
            </Link>
          }
        />
      ) : (
        <>
          <p className="mt-2 text-sm text-ink-600">
            From <Link href={`/restaurants/${cart.outlet_id}`} className="font-bold text-coral-600 hover:underline">{cart.outlet_name}</Link>{" "}
            <span className="text-ink-400">· {cart.outlet_locality} (demo)</span>
          </p>

          {hasProblems ? (
            <div className="mt-4 rounded-lg bg-marigold-50 px-4 py-3 text-sm text-ink-800" role="alert">
              <p className="font-bold">Some items changed</p>
              <p className="mt-1 text-xs text-ink-600">
                Prices and availability are revalidated against the live menu. Remove or adjust
                the affected items before ordering.
              </p>
            </div>
          ) : null}

          <Card className="mt-4">
            <CardBody className="px-0 py-0">
              <ul className="divide-y divide-ink-100">
                {cart.items.map((it) => (
                  <li key={it.id} className={cx("flex items-start justify-between gap-4 px-5 py-4", it.availability_state !== "ok" && "bg-marigold-50/50")}>
                    <div className="min-w-0">
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
                        {it.availability_state === "unavailable" || it.availability_state === "missing" ? (
                          <Badge tone="berry">Unavailable</Badge>
                        ) : null}
                        {it.availability_state === "price_changed" ? (
                          <Badge tone="marigold">
                            Now {formatINR(it.current_price_paise)}
                          </Badge>
                        ) : null}
                      </div>
                      {it.modifiers.length > 0 ? (
                        <p className="mt-1 text-xs text-ink-500">
                          {it.modifiers
                            .map((m) => `${m.group_name}: ${m.option_name}${m.price_delta_paise > 0 ? ` (+${formatINR(m.price_delta_paise, { withDecimals: false })})` : ""}`)
                            .join(" · ")}
                        </p>
                      ) : null}
                      {it.special_instructions ? (
                        <p className="mt-1 text-xs italic text-ink-500">“{it.special_instructions}”</p>
                      ) : null}
                      <p className="mt-1 text-sm font-bold text-ink-900">
                        {formatINR(it.line_total_paise)}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-col items-end gap-2">
                      <div className="flex items-center rounded-lg border border-ink-200">
                        <button
                          onClick={() => updateQty(it.id, Math.max(1, it.quantity - 1))}
                          disabled={busy || it.quantity <= 1}
                          aria-label={`Decrease quantity of ${it.name}`}
                          className="px-2.5 py-1.5 font-bold text-ink-600 hover:text-coral-600 disabled:opacity-40"
                        >
                          −
                        </button>
                        <span className="w-6 text-center text-sm font-bold">{it.quantity}</span>
                        <button
                          onClick={() => updateQty(it.id, Math.min(20, it.quantity + 1))}
                          disabled={busy}
                          aria-label={`Increase quantity of ${it.name}`}
                          className="px-2.5 py-1.5 font-bold text-ink-600 hover:text-coral-600 disabled:opacity-40"
                        >
                          +
                        </button>
                      </div>
                      <button
                        onClick={() => removeItem(it.id)}
                        disabled={busy}
                        className="text-xs font-semibold text-berry-600 hover:underline disabled:opacity-40"
                      >
                        Remove
                      </button>
                    </div>
                  </li>
                ))}
              </ul>
            </CardBody>
          </Card>

          <Card className="mt-4">
            <CardBody>
              <div className="flex items-center justify-between text-sm">
                <span className="text-ink-600">Item subtotal</span>
                <span className="font-bold text-ink-900">{formatINR(cart.items_subtotal_paise)}</span>
              </div>
              <p className="mt-2 text-xs text-ink-400">
                Taxes, delivery, platform and packaging fees are calculated precisely at checkout
                by the server — the quote you see there is the one that counts.
              </p>
              <Button
                size="lg"
                className="mt-4 w-full"
                disabled={hasProblems || busy || cart.items.length === 0}
                onClick={() => router.push("/checkout")}
              >
                Proceed to checkout
              </Button>
              {hasProblems ? (
                <p className="mt-2 text-xs text-berry-600" role="alert">
                  Resolve the flagged items to continue.
                </p>
              ) : null}
            </CardBody>
          </Card>
        </>
      )}
    </main>
  );
}

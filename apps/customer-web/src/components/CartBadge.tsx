"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { apiRequest } from "@biteorbit/api-client";
import { cx } from "@biteorbit/ui";

interface CartData {
  items: Array<{ quantity: number }>;
}

/** Header cart pill that stays fresh via the cart-updated window event. */
export function CartBadge() {
  const [count, setCount] = useState<number | null>(null);

  const refresh = useCallback(async () => {
    try {
      const cart = await apiRequest<CartData | null>("/api/v1/cart");
      setCount(cart ? cart.items.reduce((sum: number, it: { quantity: number }) => sum + it.quantity, 0) : 0);
    } catch {
      setCount(0);
    }
  }, []);

  useEffect(() => {
    refresh();
    const handler = () => refresh();
    window.addEventListener("cart-updated", handler);
    return () => window.removeEventListener("cart-updated", handler);
  }, [refresh]);

  return (
    <Link
      href="/cart"
      className="ml-2 inline-flex items-center gap-2 rounded-full bg-coral-500 px-4 py-1.5 font-semibold text-white hover:bg-coral-600 focus-visible:outline-2 focus-visible:outline-coral-500"
      aria-label={`Cart, ${count ?? 0} items`}
    >
      Cart
      <span
        aria-hidden="true"
        className={cx(
          "inline-flex h-5 min-w-5 items-center justify-center rounded-full px-1 text-xs font-bold",
          count && count > 0 ? "bg-white text-coral-600" : "bg-coral-400/60 text-white",
        )}
      >
        {count ?? "–"}
      </span>
    </Link>
  );
}

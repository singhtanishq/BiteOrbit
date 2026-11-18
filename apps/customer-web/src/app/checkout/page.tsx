import { EmptyState } from "@biteorbit/ui";

/**
 * Checkout placeholder — replaced by the full checkout flow (address,
 * quote, coupon, payment) in the pricing/checkout milestone.
 */
export default function CheckoutPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Checkout</h1>
      <EmptyState
        icon="🧾"
        title="Checkout is the very next milestone"
        description="Server-priced quotes, coupon validation and the simulated payment flow are being wired up right after the cart."
        className="mt-6"
      />
    </main>
  );
}

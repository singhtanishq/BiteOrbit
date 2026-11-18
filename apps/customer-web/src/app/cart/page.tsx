import { EmptyState } from "@biteorbit/ui";

export default function CartPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Your cart</h1>
      <EmptyState
        icon="🛒"
        title="Your cart is empty"
        description="Add dishes from Bitepur's restaurants once menu browsing is live."
        className="mt-6"
      />
    </main>
  );
}

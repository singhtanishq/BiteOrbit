import { EmptyState } from "@biteorbit/ui";

export default function OrdersPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Your orders</h1>
      <EmptyState
        icon="🧾"
        title="No orders yet"
        description="Order history and live tracking arrive with the checkout and payment milestones."
        className="mt-6"
      />
    </main>
  );
}

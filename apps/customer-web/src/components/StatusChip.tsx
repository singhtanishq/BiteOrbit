import { Badge } from "@biteorbit/ui";

const STATUS_MAP: Record<string, { tone: "leaf" | "marigold" | "berry" | "neutral" | "sky" | "coral"; label: string }> = {
  pending_payment: { tone: "sky", label: "Payment pending" },
  placed: { tone: "neutral", label: "Placed" },
  restaurant_confirmed: { tone: "sky", label: "Confirmed" },
  preparing: { tone: "marigold", label: "Preparing" },
  ready_for_pickup: { tone: "marigold", label: "Ready" },
  rider_assigned: { tone: "coral", label: "Rider assigned" },
  picked_up: { tone: "coral", label: "Picked up" },
  on_the_way: { tone: "coral", label: "On the way" },
  delivered: { tone: "leaf", label: "Delivered" },
  rejected: { tone: "berry", label: "Rejected" },
  cancel_requested: { tone: "marigold", label: "Cancelling" },
  cancelled: { tone: "neutral", label: "Cancelled" },
  delivery_failed: { tone: "berry", label: "Delivery failed" },
};

export function StatusChip({ status }: { status: string }) {
  const info = STATUS_MAP[status] ?? { tone: "neutral" as const, label: status };
  return <Badge tone={info.tone}>{info.label}</Badge>;
}

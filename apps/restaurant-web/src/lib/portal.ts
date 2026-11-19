"use client";

import { apiRequest } from "@biteorbit/api-client";

export interface PortalOutlet {
  id: string;
  name: string;
  locality: string;
  status: string;
  min_order_paise: number;
  packaging_fee_paise: number;
  prep_minutes_min: number;
  prep_minutes_max: number;
  relation: string;
}

export function listPortalOutlets() {
  return apiRequest<PortalOutlet[]>("/api/v1/restaurant-portal/outlets");
}

export interface PortalOrder {
  id: string;
  reference: string;
  status: string;
  placed_at: string;
  grand_total_paise: number;
  items_subtotal_paise: number;
  instructions?: string | null;
  items: Array<{ name: string; qty: number; unit_price_paise: number; instructions?: string | null }>;
  delivery_status: string;
}

export function listPortalOrders(outletId: string, status?: string) {
  return apiRequest<PortalOrder[]>("/api/v1/restaurant-portal/orders", {
    query: { outlet_id: outletId, status: status || undefined, limit: 50 },
  });
}

export function portalTransition(
  orderId: string,
  action: "accept" | "reject" | "prepare" | "ready",
  body: { reason?: string; prep_estimate_minutes?: number } = {},
) {
  return apiRequest<{ status: string }>(`/api/v1/restaurant-portal/orders/${orderId}/${action}`, {
    method: "POST",
    body,
  });
}

export interface PortalMenuItem {
  id: string;
  category_id: string;
  name: string;
  description: string;
  price_paise: number;
  tax_category: string;
  is_veg: boolean;
  is_available: boolean;
  is_featured: boolean;
  is_bestseller: boolean;
  allergen_notes?: string | null;
  modifier_groups: Array<{ id: string; name: string; options: Array<{ id: string; name: string }> }>;
}

export function listPortalMenu(outletId: string) {
  return apiRequest<{ categories: Array<{ id: string; name: string; items: PortalMenuItem[] }> }>(
    "/api/v1/restaurant-portal/menu",
    { query: { outlet_id: outletId } },
  );
}

export function updatePortalMenuItem(
  id: string,
  body: Partial<{ name: string; description: string; price_paise: number; is_available: boolean }>,
) {
  return apiRequest<{ updated: boolean }>(`/api/v1/restaurant-portal/menu/items/${id}`, {
    method: "PATCH",
    body,
  });
}

export interface PortalAnalytics {
  orders: number;
  delivered: number;
  cancelled: number;
  rejected: number;
  gov_paise: number;
  net_sales_paise: number;
  aov_paise: number;
  cancel_rate_bp: number;
  avg_prep_minutes: number;
  top_items: Array<{ name: string; qty: number; revenue_paise: number }>;
  data_as_of: string;
  demo_note: string;
}

export function portalAnalytics(outletId: string, days: number) {
  return apiRequest<PortalAnalytics>("/api/v1/restaurant-portal/analytics", {
    query: { outlet_id: outletId, days },
  });
}

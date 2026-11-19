import { apiRequest, ApiError } from "./http";

export { ApiError, apiRequest, buildUrl, friendlyError } from "./http";
export type { ErrorBody, RequestOptions } from "./http";

export {
  listRestaurants,
  suggestions,
  outletDetail,
  outletMenu,
  cuisines,
  collections,
  checkServiceability,
} from "./discovery";
export type {
  OutletSummary,
  OutletDetail,
  MenuItem,
  MenuCategory,
  ModifierGroup,
  ModifierOption,
  OutletMenu,
  CuisineChip,
  Suggestions,
  RestaurantFilters,
  ListMeta,
} from "./discovery";

/** Minimal authenticated-user shape returned by GET /auth/session. */
export interface SessionUser {
  id: string;
  name: string;
  email: string;
  phone?: string;
  roles: string[];
}

// --- auth & profile calls (used by every portal) -----------------------------

export async function getSession(signal?: AbortSignal) {
  return apiRequest<SessionUser>("/api/v1/auth/session", { signal });
}

export async function login(email: string, password: string) {
  return apiRequest<SessionUser>("/api/v1/auth/login", {
    method: "POST",
    body: { email, password },
  });
}

export async function register(email: string, password: string, name: string, phone?: string) {
  return apiRequest<SessionUser>("/api/v1/auth/register", {
    method: "POST",
    body: { email, password, name, phone },
  });
}

export async function logout() {
  return apiRequest<{ logged_out: boolean }>("/api/v1/auth/logout", { method: "POST" });
}

export interface Address {
  id: string;
  label: "home" | "work" | "other";
  recipient_name: string;
  recipient_phone: string;
  line1: string;
  line2?: string | null;
  landmark?: string | null;
  locality: string;
  city: string;
  state: string;
  postal_code: string;
  lat?: number | null;
  lng?: number | null;
  delivery_instructions?: string | null;
  is_default: boolean;
}

export async function listAddresses() {
  return apiRequest<Address[]>("/api/v1/me/addresses");
}

export async function createAddress(body: Omit<Address, "id" | "is_default"> & { is_default?: boolean }) {
  return apiRequest<{ id: string }>("/api/v1/me/addresses", { method: "POST", body });
}

// --- checkout calls ---------------------------------------------------------

export interface CheckoutQuoteRequest {
  address_id: string;
  coupon_code?: string;
  tip_paise?: number;
  payment_method: "cod" | "mock_upi" | "mock_card" | "mock_netbanking";
}

export interface TaxLine {
  category: string;
  rate_bp: number;
  taxable_paise: number;
  amount_paise: number;
}

export interface CheckoutBreakdown {
  items_subtotal_paise: number;
  coupon_discount_paise: number;
  packaging_fee_paise: number;
  delivery_fee_paise: number;
  platform_fee_paise: number;
  small_order_fee_paise: number;
  surge_fee_paise: number;
  tip_paise: number;
  tax_paise: number;
  tax_lines: TaxLine[];
  grand_total_paise: number;
}

export interface CheckoutQuoteResponse {
  quote_id: string;
  expires_at: string;
  outlet_id: string;
  outlet_name: string;
  address_summary: string;
  breakdown: CheckoutBreakdown;
  coupon?: { code: string; title: string; discount_paise: number; funding_party: string };
  distance_km: number;
  eta_min_minutes: number;
  eta_max_minutes: number;
  payment_method: string;
  payment_simulated: boolean;
  demo_note: string;
}

export function requestQuote(body: CheckoutQuoteRequest) {
  return apiRequest<CheckoutQuoteResponse>("/api/v1/checkout/quote", { method: "POST", body });
}

export interface PlaceOrderResponse {
  order_id: string;
  reference: string;
  status: string;
  grand_total_paise: number;
  payment_method: string;
  payment_status: string;
  payment_id: string;
  eta_min_minutes: number;
  eta_max_minutes: number;
  placed_at: string;
}

export async function placeOrder(quoteId: string, idempotencyKey: string) {
  const res = await fetch("/api/v1/checkout/place", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "Idempotency-Key": idempotencyKey,
      Accept: "application/json",
    },
    credentials: "same-origin",
    body: JSON.stringify({ quote_id: quoteId }),
  });
  const payload = await res.json();
  if (!res.ok) {
    throw new ApiError(res.status, payload?.error ?? { code: "internal_error", message: "Order placement failed." });
  }
  return payload.data as PlaceOrderResponse;
}

export interface CouponValidation {
  valid: boolean;
  code?: string;
  title?: string;
  discount_paise?: number;
  reason?: string;
  message?: string;
}

export function validateCoupon(code: string) {
  return apiRequest<CouponValidation>("/api/v1/checkout/coupon", { method: "POST", body: { code } });
}

// --- customer orders --------------------------------------------------------

export interface OrderListItem {
  id: string;
  reference: string;
  status: string;
  grand_total_paise: number;
  placed_at: string;
  eta_min_minutes: number;
  eta_max_minutes: number;
  currency: string;
  outlet_name: string;
  payment_status: string;
}

export function listOrders(page = 1) {
  return apiRequest<OrderListItem[]>("/api/v1/orders", { query: { page, limit: 10 } });
}

export interface OrderEvent {
  id: number;
  event_type: string;
  from_state?: string | null;
  to_state?: string | null;
  actor_type: string;
  reason?: string | null;
  created_at: string;
}

export interface OrderDetail {
  id: string;
  reference: string;
  status: string;
  placed_at: string;
  eta_min_minutes: number;
  eta_max_minutes: number;
  outlet: { id: string; name: string; phone: string };
  breakdown: CheckoutBreakdown & { pricing_rule_version: number };
  items: Array<{
    id: string;
    name: string;
    is_veg: boolean;
    quantity: number;
    unit_price_paise: number;
    item_total_paise: number;
    special_instructions?: string | null;
    modifiers: Array<{ group: string; option: string; price_delta_paise: number }>;
  }>;
  events: OrderEvent[];
  payment: { present: boolean; payment: PaymentView };
  refunds: Array<{ id: string; amount_paise: number; status: string; reason: string; created_at: string }>;
  address?: string;
  rider?: { name: string; phone?: string | null };
  instructions?: string | null;
  can_cancel: boolean;
}

export interface PaymentView {
  id: string;
  method: string;
  status: string;
  amount_paise: number;
  failure_reason?: string | null;
  simulated: boolean;
}

export function getOrder(id: string) {
  return apiRequest<OrderDetail>(`/api/v1/orders/${id}`);
}

export function cancelOrder(id: string, reason: string) {
  return apiRequest<{ status: string; refund_initiated: boolean }>(`/api/v1/orders/${id}/cancel`, {
    method: "POST",
    body: { reason },
  });
}

export function simulatePayment(paymentId: string, outcome: "success" | "failure") {
  return apiRequest<{ payment_id: string; status: string; simulated: boolean; demo_note: string }>(
    `/api/v1/payments/${paymentId}/simulate`,
    { method: "POST", body: { outcome } },
  );
}

export function requestRefund(orderId: string, reason: string) {
  return apiRequest<{ refund_status: string; simulated: boolean }>(`/api/v1/orders/${orderId}/refund`, {
    method: "POST",
    body: { reason },
  });
}

// --- P1: reviews, favourites, reorder, support, notifications ----------------

export async function createReview(orderId: string, rating: number, comment: string, deliveryRating?: number) {
  return apiRequest<{ id: string; outlet_id: string }>("/api/v1/reviews", {
    method: "POST",
    body: { order_id: orderId, rating, delivery_rating: deliveryRating ?? 0, comment },
  });
}

export interface OutletReview {
  rating: number;
  delivery_rating: number;
  comment?: string | null;
  created_at: string;
  customer_name: string;
}

export function outletReviews(id: string, page = 1) {
  return apiRequest<OutletReview[]>(`/api/v1/restaurants/${id}/reviews`, { query: { page, limit: 10 } });
}

export function listFavourites() {
  return apiRequest<Array<{ id: string; name: string; locality: string; city: string; rating: number; is_pure_veg: boolean }>>(
    "/api/v1/me/favourites",
  );
}

export function favouriteOutlet(outletId: string) {
  return apiRequest<{ favourited: boolean }>(`/api/v1/me/favourites/${outletId}`, { method: "POST" });
}

export function unfavouriteOutlet(outletId: string) {
  return apiRequest<{ removed: boolean }>(`/api/v1/me/favourites/${outletId}`, { method: "DELETE" });
}

export function reorder(orderId: string) {
  return apiRequest<{ cart_id: string; added: number; skipped: number; skipped_reason?: string; replaced_other_outlet_cart: boolean }>(
    `/api/v1/orders/${orderId}/reorder`,
    { method: "POST" },
  );
}

export interface SupportTicketRow {
  id: string;
  reference: string;
  category: string;
  subject: string;
  status: string;
  priority: string;
  created_at: string;
  updated_at: string;
  replies: number;
}

export function createSupportTicket(category: string, subject: string, body: string, orderId?: string) {
  return apiRequest<{ id: string; reference: string; message: string }>("/api/v1/support/tickets", {
    method: "POST",
    body: { category, subject, body, order_id: orderId || undefined },
  });
}

export function mySupportTickets() {
  return apiRequest<SupportTicketRow[]>("/api/v1/support/tickets");
}

export interface SupportThread {
  ticket: { id: string; reference: string; category: string; subject: string; status: string; created_at: string };
  messages: Array<{ id: string; author_type: string; body: string; created_at: string }>;
}

export function supportThread(id: string) {
  return apiRequest<SupportThread>(`/api/v1/support/tickets/${id}`);
}

export function replySupportTicket(id: string, body: string) {
  return apiRequest<{ replied: boolean }>(`/api/v1/support/tickets/${id}/messages`, {
    method: "POST",
    body: { body },
  });
}

export interface AppNotification {
  id: string;
  category: string;
  title: string;
  body: string;
  read_at?: string | null;
  created_at: string;
}

export function listNotifications(unreadOnly?: boolean) {
  return apiRequest<AppNotification[]>("/api/v1/notifications", {
    query: { unread_only: unreadOnly ? "true" : undefined },
  });
}

export function markNotificationRead(id: string) {
  return apiRequest<{ read: boolean }>(`/api/v1/notifications/${id}/read`, { method: "POST" });
}

export function markAllNotificationsRead() {
  return apiRequest<{ read: boolean }>("/api/v1/notifications/read-all", { method: "POST" });
}

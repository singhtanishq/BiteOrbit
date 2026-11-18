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

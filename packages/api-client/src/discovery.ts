/**
 * Discovery API types and calls (shared by all frontends).
 * Mirrors the Go discovery DTOs; money fields are integer paise.
 */
import { apiRequest } from "./http";

export interface OutletSummary {
  id: string;
  name: string;
  description?: string;
  locality: string;
  city: string;
  cuisines: string[];
  price_band: number;
  min_order_paise: number;
  is_pure_veg: boolean;
  prep_minutes_min: number;
  prep_minutes_max: number;
  rating: number;
  review_count: number;
  distance_km?: number;
  is_open: boolean;
  closed_reason?: string;
  image_key?: string;
}

export interface ListMeta {
  page: number;
  limit: number;
  total: number;
}

export interface OutletDetail extends OutletSummary {
  phone: string;
  organisation: string;
  address: {
    line1: string;
    locality: string;
    city: string;
    state: string;
    postal_code: string;
    lat: number;
    lng: number;
  };
  hours: Array<{ weekday: number; opens_at: string; closes_at: string; is_closed: boolean }>;
  collections: string[];
}

export interface ModifierOption {
  id: string;
  name: string;
  price_delta_paise: number;
  is_default: boolean;
  is_available: boolean;
}

export interface ModifierGroup {
  id: string;
  name: string;
  is_required: boolean;
  min_select: number;
  max_select: number;
  options: ModifierOption[];
}

export interface MenuItem {
  id: string;
  category_id: string;
  name: string;
  description: string;
  price_paise: number;
  tax_category: string;
  is_veg: boolean;
  dietary_tags: string[];
  allergen_notes: string | null;
  serving_info: string | null;
  image_key: string | null;
  is_featured: boolean;
  is_bestseller: boolean;
  is_available: boolean;
  modifier_groups: ModifierGroup[];
}

export interface MenuCategory {
  id: string;
  name: string;
  display_order: number;
  items: MenuItem[];
}

export interface OutletMenu {
  outlet_id: string;
  outlet_name: string;
  categories: MenuCategory[];
}

export interface CuisineChip {
  name: string;
  slug: string;
  outlets: number;
}

export interface Suggestions {
  restaurants: Array<{ id: string; name: string; locality: string }>;
  cuisines: Array<{ name: string; slug: string }>;
  dishes: Array<{ id: string; name: string; outlet: string; is_veg: boolean }>;
}

export interface RestaurantFilters {
  query?: string;
  cuisine?: string;
  collection?: string;
  veg?: string;
  price_min?: number;
  price_max?: number;
  sort?: string;
  page?: number;
  limit?: number;
  lat?: number;
  lng?: number;
}

export function listRestaurants(filters: RestaurantFilters = {}, signal?: AbortSignal) {
  return apiRequest<OutletSummary[]>("/api/v1/discovery/restaurants", {
    query: { limit: 50, ...filters } as RestaurantFilters as Record<string, string | number>,
    signal,
  });
}

export function listRestaurantsWithMeta(
  filters: RestaurantFilters = {},
  signal?: AbortSignal,
): Promise<{ data: OutletSummary[]; meta?: ListMeta }> {
  // The envelope separates data/meta; apiRequest returns data only, so call
  // the low-level request and re-read meta via a second pass on the same
  // response is unnecessary — instead return both through a typed wrapper.
  return rawList(filters, signal);
}

async function rawList(
  filters: RestaurantFilters,
  signal?: AbortSignal,
): Promise<{ data: OutletSummary[]; meta?: ListMeta }> {
  const res = await fetch(withQuery("/api/v1/discovery/restaurants", {
    limit: 50,
    ...filters,
  }), { signal, credentials: "same-origin" });
  if (!res.ok) {
    throw new Error(`list failed: ${res.status}`);
  }
  const payload = (await res.json()) as { data: OutletSummary[]; meta?: ListMeta };
  return { data: payload.data, meta: payload.meta };
}

function withQuery(path: string, params: Record<string, string | number | undefined>): string {
  const usp = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") usp.set(k, String(v));
  }
  const qs = usp.toString();
  return qs ? `${path}?${qs}` : path;
}

export function suggestions(q: string, signal?: AbortSignal) {
  return apiRequest<Suggestions>("/api/v1/discovery/suggestions", { query: { q }, signal });
}

export function outletDetail(id: string, signal?: AbortSignal) {
  return apiRequest<OutletDetail>(`/api/v1/restaurants/${id}`, { signal });
}

export function outletMenu(id: string, signal?: AbortSignal) {
  return apiRequest<OutletMenu>(`/api/v1/restaurants/${id}/menu`, { signal });
}

export function cuisines(signal?: AbortSignal) {
  return apiRequest<CuisineChip[]>("/api/v1/discovery/cuisines", { signal });
}

export function collections(signal?: AbortSignal) {
  return apiRequest<Array<{ name: string; outlets: number }>>("/api/v1/discovery/collections", { signal });
}

export function checkServiceability(lat: number, lng: number) {
  return apiRequest<{ serviceable: boolean; zones: Array<{ name: string; distance_km?: number }> }>(
    "/api/v1/discovery/serviceability",
    { method: "POST", body: { lat, lng } },
  );
}

import { apiRequest } from "./http";

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

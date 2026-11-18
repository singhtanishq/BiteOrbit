export { ApiError, apiRequest, buildUrl, friendlyError } from "./http";
export type { ErrorBody, RequestOptions } from "./http";

/** Minimal authenticated-user shape returned by GET /auth/session. */
export interface SessionUser {
  id: string;
  name: string;
  email: string;
  phone?: string;
  roles: string[];
}

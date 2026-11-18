/**
 * Shared browser API client for all BiteOrbit apps.
 *
 * In every app, requests go through a same-origin Next.js rewrite to the Go API
 * (`/api/v1/...` → API server), so cookie sessions work without cross-site CORS.
 */

export interface ErrorBody {
  code: string;
  message: string;
  details?: Record<string, unknown>;
  requestId?: string;
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details?: Record<string, unknown>;
  readonly requestId?: string;

  constructor(status: number, body: ErrorBody) {
    super(body.message);
    this.name = "ApiError";
    this.status = status;
    this.code = body.code;
    this.details = body.details;
    this.requestId = body.requestId;
  }
}

export interface RequestOptions {
  method?: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
  body?: unknown;
  query?: Record<string, string | number | boolean | undefined>;
  signal?: AbortSignal;
  idempotencyKey?: string;
}

export function buildUrl(path: string, query?: RequestOptions["query"]): string {
  const url = new URL(path, "http://internal");
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined) url.searchParams.set(key, String(value));
    }
  }
  return url.pathname + (url.search ? url.search : "");
}

/** Perform an API request against the same-origin proxy. Returns parsed `data`. */
export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const { method = "GET", body, query, signal, idempotencyKey } = opts;
  const headers: Record<string, string> = { Accept: "application/json" };
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (idempotencyKey) headers["Idempotency-Key"] = idempotencyKey;

  const res = await fetch(buildUrl(path, query), {
    method,
    headers,
    credentials: "same-origin",
    signal,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const text = await res.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      payload = null;
    }
  }

  if (!res.ok) {
    const envelope = payload as { error?: ErrorBody } | null;
    const errBody = envelope?.error ?? {
      code: "internal_error",
      message: `Request failed with status ${res.status}`,
    };
    throw new ApiError(res.status, errBody);
  }

  const envelope = payload as { data?: T } | null;
  return (envelope?.data ?? (payload as T)) as T;
}

/** Format an ApiError (or anything) for user-facing messages without leaking internals. */
export function friendlyError(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof DOMException && err.name === "AbortError") return "Request timed out.";
  return "Something went wrong. Please try again.";
}

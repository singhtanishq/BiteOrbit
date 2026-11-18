import { describe, expect, it } from "vitest";
import { ApiError, buildUrl } from "./http";

describe("buildUrl", () => {
  it("adds query params skipping undefined", () => {
    expect(buildUrl("/api/v1/orders", { page: 2, q: "dosa", empty: undefined })).toBe(
      "/api/v1/orders?page=2&q=dosa",
    );
  });

  it("handles no query", () => {
    expect(buildUrl("/api/v1/me")).toBe("/api/v1/me");
  });
});

describe("ApiError", () => {
  it("exposes code and message", () => {
    const err = new ApiError(403, { code: "forbidden", message: "Not allowed" });
    expect(err.status).toBe(403);
    expect(err.code).toBe("forbidden");
    expect(err.message).toBe("Not allowed");
  });
});

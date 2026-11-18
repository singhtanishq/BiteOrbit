import { describe, expect, it } from "vitest";
import { formatINR, rupeesToPaise } from "./money";
import { cx } from "./cx";

describe("formatINR", () => {
  it("formats paise with decimals", () => {
    expect(formatINR(123456)).toBe("₹1,234.56");
  });

  it("formats whole rupees without decimals when asked", () => {
    expect(formatINR(123400, { withDecimals: false })).toBe("₹1,234");
  });

  it("keeps trailing zero paise", () => {
    expect(formatINR(500)).toBe("₹5.00");
  });

  it("handles negative amounts", () => {
    expect(formatINR(-1250)).toBe("-₹12.50");
  });
});

describe("rupeesToPaise", () => {
  it("parses plain rupee strings", () => {
    expect(rupeesToPaise("12.34")).toBe(1234);
    expect(rupeesToPaise("12")).toBe(1200);
  });

  it("accepts symbol and thousand separators", () => {
    expect(rupeesToPaise("₹1,234.5")).toBe(123450);
  });

  it("rejects invalid input", () => {
    expect(rupeesToPaise("abc")).toBeNull();
    expect(rupeesToPaise("1.234")).toBeNull();
    expect(rupeesToPaise("")).toBeNull();
  });
});

describe("cx", () => {
  it("drops falsy classes", () => {
    expect(cx("a", false, undefined, "b", null)).toBe("a b");
  });
});

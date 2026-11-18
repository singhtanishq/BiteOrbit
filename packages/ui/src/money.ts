/**
 * Money helpers. All BiteOrbit amounts are integer paise (1 INR = 100 paise).
 * This module is the single place that converts paise to display strings.
 */
export function formatINR(paise: number, opts?: { withDecimals?: boolean }): string {
  const negative = paise < 0;
  const abs = Math.abs(Math.round(paise));
  const rupees = Math.floor(abs / 100);
  const rest = abs % 100;
  const withDecimals = opts?.withDecimals ?? true;
  const rupeeStr = rupees.toLocaleString("en-IN");
  if (!withDecimals && rest === 0) {
    return `${negative ? "-" : ""}₹${rupeeStr}`;
  }
  return `${negative ? "-" : ""}₹${rupeeStr}.${String(rest).padStart(2, "0")}`;
}

/** Parse a user-entered rupee string into paise, rounding to the nearest paise. */
export function rupeesToPaise(input: string): number | null {
  const trimmed = input.trim().replace(/[₹,\s]/g, "");
  if (!/^\d+(\.\d{1,2})?$/.test(trimmed)) return null;
  const [whole, frac = ""] = trimmed.split(".");
  const fracPadded = (frac + "00").slice(0, 2);
  return Number(whole) * 100 + Number(fracPadded || "0");
}

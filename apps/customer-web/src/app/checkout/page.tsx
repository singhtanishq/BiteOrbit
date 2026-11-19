"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import {
  Address,
  ApiError,
  CheckoutQuoteResponse,
  friendlyError,
  listAddresses,
  placeOrder,
  PlaceOrderResponse,
  requestQuote,
  validateCoupon,
  CouponValidation,
} from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, EmptyState, Input, Select, Spinner, cx, formatINR } from "@biteorbit/ui";

type PaymentMethod = "cod" | "mock_upi" | "mock_card";

const PAYMENT_OPTIONS: Array<{ value: PaymentMethod; label: string; hint: string }> = [
  { value: "cod", label: "Cash on delivery", hint: "Pay the rider at your door." },
  { value: "mock_upi", label: "UPI (simulated)", hint: "Demo payment — no real money moves." },
  { value: "mock_card", label: "Card (simulated)", hint: "Demo payment — no card details are stored." },
];

const TIPS = [0, 2000, 3000, 5000];

export default function CheckoutPage() {
  const router = useRouter();
  const [addresses, setAddresses] = useState<Address[] | null>(null);
  const [addressId, setAddressId] = useState("");
  const [method, setMethod] = useState<PaymentMethod>("cod");
  const [tipPaise, setTipPaise] = useState(0);
  const [couponInput, setCouponInput] = useState("");
  const [couponState, setCouponState] = useState<CouponValidation | null>(null);
  const [quote, setQuote] = useState<CheckoutQuoteResponse | null>(null);
  const [placed, setPlaced] = useState<PlaceOrderResponse | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  const refreshAddresses = useCallback(async () => {
    try {
      const rows = await listAddresses();
      setAddresses(rows);
      const def = rows.find((a) => a.is_default) ?? rows[0];
      if (def) setAddressId((prev) => prev || def.id);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setError("Please sign in from the Account page to check out.");
      } else {
        setError(friendlyError(err));
      }
      setAddresses([]);
    }
  }, []);

  useEffect(() => {
    refreshAddresses();
  }, [refreshAddresses]);

  const getQuote = useCallback(async () => {
    setError("");
    setBusy(true);
    try {
      const q = await requestQuote({
        address_id: addressId,
        coupon_code: couponState?.valid ? couponState.code : undefined,
        tip_paise: tipPaise,
        payment_method: method,
      });
      setQuote(q);
    } catch (err) {
      setQuote(null);
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  }, [addressId, couponState, method, tipPaise]);

  // re-quote whenever any input changes (debounced by user action)
  useEffect(() => {
    if (placed || !addressId) return;
    getQuote();
  }, [addressId, method, tipPaise, couponState, placed, getQuote]);

  const applyCoupon = async () => {
    setError("");
    if (!couponInput.trim()) return;
    setBusy(true);
    try {
      const res = await validateCoupon(couponInput.trim());
      setCouponState(res);
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const removeCoupon = () => {
    setCouponInput("");
    setCouponState(null);
  };

  const confirmPlace = async () => {
    if (!quote) return;
    setBusy(true);
    setError("");
    try {
      // one key per checkout attempt: retries after failures reuse it
      const key = `web-${quote.quote_id}-${Date.now()}`;
      const order = await placeOrder(quote.quote_id, key);
      setPlaced(order);
      window.dispatchEvent(new CustomEvent("cart-updated"));
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  if (placed) {
    return (
      <main className="mx-auto max-w-2xl px-4 py-12">
        <Card className="text-center">
          <CardBody>
            <p className="text-5xl" aria-hidden="true">🎉</p>
            <h1 className="mt-3 text-2xl font-extrabold text-ink-900">Order placed!</h1>
            <p className="mt-1 text-sm text-ink-600">
              Reference <strong>{placed.reference}</strong> ·{" "}
              {placed.payment_method === "cod"
                ? "Pay on delivery"
                : "Payment pending (simulated gateway)"}
            </p>
            <p className="mt-1 text-xs text-ink-500">
              Estimated delivery in {placed.eta_min_minutes}–{placed.eta_max_minutes} minutes
              (demo estimate).
            </p>
            <p className="mt-4 text-sm font-bold">{formatINR(placed.grand_total_paise)}</p>
            <div className="mt-6 flex justify-center gap-3">
              <Button onClick={() => router.push("/orders")}>Track this order</Button>
              <Button variant="secondary" onClick={() => router.push("/")}>
                Order something else
              </Button>
            </div>
            <p className="mt-4 text-[11px] text-ink-400">
              Demo note: this order is synthetic and no real fulfilment will occur.
            </p>
          </CardBody>
        </Card>
      </main>
    );
  }

  const hasAddresses = (addresses?.length ?? 0) > 0;

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Checkout</h1>

      {error ? (
        <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm font-medium text-berry-700">
          {error}
        </p>
      ) : null}

      {addresses !== null && !hasAddresses && !error ? (
        <EmptyState
          icon="🏠"
          title="No delivery address"
          description="Add an address in a demo locality (e.g., Rosewood Hills) to continue."
          className="mt-6"
          action={
            <Link href="/account">
              <Button>Add address</Button>
            </Link>
          }
        />
      ) : null}

      {hasAddresses ? (
        <div className="mt-6 space-y-4">
          {/* address */}
          <Card>
            <CardHeader>
              <CardTitle>Delivery address</CardTitle>
            </CardHeader>
            <CardBody>
              <Select
                label="Choose an address"
                value={addressId}
                onChange={(e) => setAddressId(e.target.value)}
              >
                {addresses?.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.label} — {a.line1}, {a.locality} ({a.recipient_name})
                  </option>
                ))}
              </Select>
              <p className="mt-2 text-xs text-ink-500">
                Manage addresses in the{" "}
                <Link href="/account" className="font-semibold text-coral-600 hover:underline">
                  Account
                </Link>{" "}
                page. Serviceability is validated against the demo city zones.
              </p>
            </CardBody>
          </Card>

          {/* coupon */}
          <Card>
            <CardHeader>
              <CardTitle>Coupon</CardTitle>
            </CardHeader>
            <CardBody>
              {couponState?.valid ? (
                <div className="flex items-center justify-between rounded-lg bg-leaf-50 px-4 py-3">
                  <div className="text-sm">
                    <span className="font-bold text-leaf-700">{couponState.code}</span> applied —{" "}
                    <strong>−{formatINR(couponState.discount_paise ?? 0)}</strong>{" "}
                    <span className="text-ink-500">{couponState.title}</span>
                  </div>
                  <Button size="sm" variant="ghost" onClick={removeCoupon}>
                    Remove
                  </Button>
                </div>
              ) : (
                <>
                  <div className="flex gap-2">
                    <Input
                      value={couponInput}
                      onChange={(e) => setCouponInput(e.target.value.toUpperCase())}
                      placeholder="e.g., ORBIT50"
                      aria-label="Coupon code"
                    />
                    <Button variant="secondary" onClick={applyCoupon} loading={busy}>
                      Apply
                    </Button>
                  </div>
                  {couponState && !couponState.valid ? (
                    <p className="mt-2 text-xs font-medium text-berry-600" role="alert">
                      {couponState.message}
                    </p>
                  ) : null}
                  <p className="mt-2 text-xs text-ink-400">
                    Demo codes: ORBIT50, FLY20, NEWBITE (first order), TANDOOR15.
                  </p>
                </>
              )}
            </CardBody>
          </Card>

          {/* tip */}
          <Card>
            <CardHeader>
              <CardTitle>Tip your delivery partner</CardTitle>
            </CardHeader>
            <CardBody>
              <div className="flex gap-2" role="group" aria-label="Tip amount">
                {TIPS.map((t) => (
                  <button
                    key={t}
                    onClick={() => setTipPaise(t)}
                    aria-pressed={tipPaise === t}
                    className={cx(
                      "rounded-full border px-4 py-1.5 text-sm font-semibold transition-colors",
                      tipPaise === t
                        ? "border-coral-500 bg-coral-50 text-coral-700"
                        : "border-ink-200 bg-white text-ink-600",
                    )}
                  >
                    {t === 0 ? "No tip" : formatINR(t, { withDecimals: false })}
                  </button>
                ))}
              </div>
              <p className="mt-2 text-xs text-ink-400">
                100% of the tip goes to the rider (demo ledger).
              </p>
            </CardBody>
          </Card>

          {/* payment */}
          <Card>
            <CardHeader>
              <CardTitle>Payment method</CardTitle>
            </CardHeader>
            <CardBody>
              <div className="space-y-2">
                {PAYMENT_OPTIONS.map((opt) => (
                  <label
                    key={opt.value}
                    className={cx(
                      "flex cursor-pointer items-start gap-3 rounded-lg border px-4 py-3 transition-colors",
                      method === opt.value ? "border-coral-500 bg-coral-50" : "border-ink-200 bg-white",
                    )}
                  >
                    <input
                      type="radio"
                      name="payment"
                      checked={method === opt.value}
                      onChange={() => setMethod(opt.value)}
                      className="mt-1 h-4 w-4 accent-[color:var(--color-coral-500)]"
                    />
                    <span>
                      <span className="block text-sm font-bold text-ink-900">{opt.label}</span>
                      <span className="block text-xs text-ink-500">{opt.hint}</span>
                    </span>
                  </label>
                ))}
              </div>
            </CardBody>
          </Card>

          {/* quote breakdown */}
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <CardTitle>Order summary</CardTitle>
                {quote ? <Badge tone="leaf">Quote valid 10 min</Badge> : null}
              </div>
            </CardHeader>
            <CardBody>
              {busy && !quote ? (
                <div className="flex justify-center py-6">
                  <Spinner label="Calculating prices" />
                </div>
              ) : quote ? (
                <>
                  <p className="text-xs text-ink-500">
                    {quote.outlet_name} → {quote.address_summary} · ~{Math.round(quote.distance_km * 10) / 10} km
                    (straight-line estimate) · delivery in {quote.eta_min_minutes}–
                    {quote.eta_max_minutes} min (demo estimate)
                  </p>
                  <dl className="mt-4 space-y-1.5 text-sm">
                    <Row label="Item subtotal" value={formatINR(quote.breakdown.items_subtotal_paise)} />
                    {quote.breakdown.coupon_discount_paise > 0 ? (
                      <Row
                        label={`Coupon (${quote.coupon?.code ?? "applied"})`}
                        value={`−${formatINR(quote.breakdown.coupon_discount_paise)}`}
                        tone="leaf"
                      />
                    ) : null}
                    <Row label="Packaging" value={formatINR(quote.breakdown.packaging_fee_paise)} />
                    <Row
                      label="Delivery fee"
                      value={formatINR(quote.breakdown.delivery_fee_paise)}
                      muted={quote.breakdown.delivery_fee_paise === 0}
                    />
                    <Row label="Platform fee" value={formatINR(quote.breakdown.platform_fee_paise)} />
                    {quote.breakdown.small_order_fee_paise > 0 ? (
                      <Row label="Small-order fee" value={formatINR(quote.breakdown.small_order_fee_paise)} />
                    ) : null}
                    {quote.breakdown.tip_paise > 0 ? (
                      <Row label="Tip" value={formatINR(quote.breakdown.tip_paise)} />
                    ) : null}
                    {quote.breakdown.tax_lines.map((tl) => (
                      <Row
                        key={tl.category}
                        label={`GST (${tl.category.replace("gst_", "")}% on ${formatINR(tl.taxable_paise, { withDecimals: false })})`}
                        value={formatINR(tl.amount_paise)}
                      />
                    ))}
                  </dl>
                  <div className="mt-4 flex items-center justify-between border-t border-ink-100 pt-3">
                    <span className="text-base font-extrabold">To pay</span>
                    <span className="text-lg font-extrabold text-ink-900">
                      {formatINR(quote.breakdown.grand_total_paise)}
                    </span>
                  </div>
                  <p className="mt-2 text-[11px] text-ink-400">
                    {quote.demo_note} Every amount here is computed server-side and snapshotted to
                    your order — later price-rule changes never alter placed orders.
                  </p>
                  <Button size="lg" className="mt-4 w-full" loading={busy} onClick={confirmPlace}>
                    Place order · {formatINR(quote.breakdown.grand_total_paise)}
                  </Button>
                </>
              ) : null}
            </CardBody>
          </Card>
        </div>
      ) : null}
    </main>
  );
}

function Row({
  label,
  value,
  tone,
  muted,
}: {
  label: string;
  value: string;
  tone?: "leaf";
  muted?: boolean;
}) {
  return (
    <div className="flex items-center justify-between">
      <dt className="text-ink-600">{label}</dt>
      <dd
        className={cx(
          "font-semibold",
          tone === "leaf" ? "text-leaf-700" : muted ? "text-leaf-600" : "text-ink-900",
        )}
      >
        {value}
      </dd>
    </div>
  );
}

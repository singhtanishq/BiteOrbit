"use client";

import { useCallback, useEffect, useState } from "react";
import { adminCreateCoupon, adminCoupons, adminUpdateCoupon, AdminCouponRow } from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Badge, Button, Card, CardBody, EmptyState, Input, Select, Skeleton, formatINR } from "@biteorbit/ui";

export default function CouponsPage() {
  const { ready } = useAdminScope();
  const [rows, setRows] = useState<AdminCouponRow[] | null>(null);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [creating, setCreating] = useState(false);
  const [toggling, setToggling] = useState<AdminCouponRow | null>(null);
  const [reason, setReason] = useState("");
  const [form, setForm] = useState({
    code: "", title: "", discount_type: "percent", percent_bp: "1000",
    fixed_paise: "5000", min_order_paise: "29900", valid_days: "30",
    usage_limit_per_user: "1", total_redemption_limit: "", new_users_only: false,
  });

  const load = useCallback(async () => {
    setError("");
    try {
      setRows(await adminCoupons());
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, []);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const create = async () => {
    setBusy(true);
    setError("");
    try {
      const body: Record<string, unknown> = {
        code: form.code.trim().toUpperCase(),
        title: form.title.trim(),
        discount_type: form.discount_type,
        min_order_paise: Math.round(parseFloat(form.min_order_paise || "0") * 100),
        valid_days: parseInt(form.valid_days || "30", 10),
        usage_limit_per_user: parseInt(form.usage_limit_per_user || "1", 10),
        new_users_only: form.new_users_only,
      };
      if (form.discount_type === "percent") {
        body.percent_bp = Math.round(parseFloat(form.percent_bp) * 100);
      } else {
        body.fixed_paise = Math.round(parseFloat(form.fixed_paise) * 100);
      }
      if (form.total_redemption_limit) {
        body.total_redemption_limit = parseInt(form.total_redemption_limit, 10);
      }
      await adminCreateCoupon(body);
      setCreating(false);
      setNotice(`Coupon ${form.code.toUpperCase()} created (audited).`);
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Create failed");
    } finally {
      setBusy(false);
    }
  };

  const toggle = async () => {
    if (!toggling) return;
    setBusy(true);
    try {
      await adminUpdateCoupon(toggling.id, !toggling.is_active, reason.trim());
      setToggling(null);
      setReason("");
      setNotice("Coupon updated (audited).");
      await load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <AdminShell active="coupons" title="Coupons & promotions" subtitle="Funding party, caps and usage limits are enforced server-side at quote and placement.">
      <div className="flex items-center justify-between">
        <Button onClick={() => setCreating(true)}>New coupon</Button>
      </div>
      {notice ? <p role="status" className="mt-4 rounded-lg bg-leaf-50 px-4 py-3 text-sm text-leaf-700">{notice}</p> : null}
      {error ? <p role="alert" className="mt-2 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}

      {!rows ? (
        <Skeleton className="mt-4 h-56 w-full" />
      ) : rows.length === 0 ? (
        <EmptyState icon="🎟️" title="No coupons" className="mt-4" />
      ) : (
        <ul className="mt-4 space-y-3">
          {rows.map((c) => (
            <li key={c.id}>
              <Card>
                <CardBody>
                  <div className="flex flex-wrap items-center justify-between gap-2">
                    <div className="flex items-center gap-2">
                      <span className="font-extrabold">{c.code}</span>
                      <Badge tone={c.is_active ? "leaf" : "neutral"}>{c.is_active ? "active" : "inactive"}</Badge>
                      {c.new_users_only ? <Badge tone="sky">new users</Badge> : null}
                      <Badge tone="neutral">{c.funding_party} funded</Badge>
                    </div>
                    <Button size="sm" variant="ghost" onClick={() => setToggling(c)}>
                      {c.is_active ? "Deactivate…" : "Activate…"}
                    </Button>
                  </div>
                  <p className="mt-1 text-sm text-ink-600">{c.title}</p>
                  <p className="mt-1 text-xs text-ink-500">
                    {c.discount_type === "percent"
                      ? `${((c.percent_bp ?? 0) / 100).toFixed(1)}% off`
                      : `${formatINR(c.fixed_paise ?? 0)} off`}
                    {c.min_order_paise > 0 ? ` · min ${formatINR(c.min_order_paise, { withDecimals: false })}` : ""}
                    {` · ${c.redeemed_count} redeemed${c.usage_limit_per_user ? ` · ${c.usage_limit_per_user}/user` : ""}`}
                  </p>
                </CardBody>
              </Card>
            </li>
          ))}
        </ul>
      )}

      {creating ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center overflow-y-auto bg-ink-950/50 p-4" role="dialog" aria-modal="true">
          <div className="my-8 w-full max-w-lg rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">Create coupon</h2>
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <Input label="Code" value={form.code} onChange={(e) => setForm({ ...form, code: e.target.value.toUpperCase() })} />
              <Input label="Title" value={form.title} onChange={(e) => setForm({ ...form, title: e.target.value })} />
              <Select label="Type" value={form.discount_type} onChange={(e) => setForm({ ...form, discount_type: e.target.value })}>
                <option value="percent">Percent</option>
                <option value="fixed">Fixed amount</option>
              </Select>
              {form.discount_type === "percent" ? (
                <Input label="Discount %" value={form.percent_bp} onChange={(e) => setForm({ ...form, percent_bp: e.target.value })} hint="e.g., 10 = 10%" />
              ) : (
                <Input label="Discount (₹)" value={form.fixed_paise} onChange={(e) => setForm({ ...form, fixed_paise: e.target.value })} />
              )}
              <Input label="Min order (₹)" value={form.min_order_paise} onChange={(e) => setForm({ ...form, min_order_paise: e.target.value })} />
              <Input label="Valid days" value={form.valid_days} onChange={(e) => setForm({ ...form, valid_days: e.target.value })} />
              <Input label="Per-user limit" value={form.usage_limit_per_user} onChange={(e) => setForm({ ...form, usage_limit_per_user: e.target.value })} />
              <Input label="Total limit (optional)" value={form.total_redemption_limit} onChange={(e) => setForm({ ...form, total_redemption_limit: e.target.value })} />
              <label className="flex items-center gap-2 text-sm sm:col-span-2">
                <input type="checkbox" checked={form.new_users_only} onChange={(e) => setForm({ ...form, new_users_only: e.target.checked })} />
                New users only
              </label>
            </div>
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setCreating(false)}>Cancel</Button>
              <Button onClick={create} disabled={!form.code || !form.title} loading={busy}>Create</Button>
            </div>
          </div>
        </div>
      ) : null}

      {toggling ? (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-ink-950/50 p-4" role="dialog" aria-modal="true">
          <div className="w-full max-w-md rounded-2xl bg-white p-6 shadow-pop">
            <h2 className="text-lg font-extrabold">
              {toggling.is_active ? "Deactivate" : "Activate"} {toggling.code}
            </h2>
            <textarea
              value={reason}
              onChange={(e) => setReason(e.target.value)}
              rows={3}
              aria-label="Reason"
              placeholder="Mandatory reason (audited)…"
              className="mt-3 w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
            />
            <div className="mt-4 flex justify-end gap-2">
              <Button variant="ghost" onClick={() => setToggling(null)}>Back</Button>
              <Button
                variant={toggling.is_active ? "danger" : "primary"}
                onClick={toggle}
                disabled={reason.trim().length < 4}
                loading={busy}
              >
                Confirm
              </Button>
            </div>
          </div>
        </div>
      ) : null}
    </AdminShell>
  );
}

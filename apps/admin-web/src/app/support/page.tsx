"use client";

import { useCallback, useEffect, useState } from "react";
import {
  adminReplyTicket,
  adminTicketDetail,
  adminTickets,
  adminUpdateTicket,
  AdminTicketRow,
  AdminTicketThread,
} from "@/lib/admin";
import { AdminShell, useAdminScope } from "@/components/AdminShell";
import { Badge, Button, Card, CardBody, EmptyState, Select, Skeleton, cx } from "@biteorbit/ui";

export default function SupportPage() {
  const { ready } = useAdminScope();
  const [tickets, setTickets] = useState<AdminTicketRow[] | null>(null);
  const [thread, setThread] = useState<AdminTicketThread | null>(null);
  const [statusFilter, setStatusFilter] = useState("");
  const [threadError, setThreadError] = useState("");
  const [error, setError] = useState("");
  const [reply, setReply] = useState("");
  const [internal, setInternal] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setTickets(await adminTickets(statusFilter || undefined));
      setError("");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load");
    }
  }, [statusFilter]);

  useEffect(() => {
    if (ready) load();
  }, [ready, load]);

  const openThread = async (id: string) => {
    try {
      setThread(await adminTicketDetail(id));
    } catch (err) {
      setThreadError(err instanceof Error ? err.message : "Failed to load thread");
    }
  };

  const sendReply = async () => {
    if (!thread || reply.trim().length < 2) return;
    setBusy(true);
    try {
      await adminReplyTicket(thread.ticket.id, reply.trim(), internal);
      setReply("");
      setInternal(false);
      setThread(await adminTicketDetail(thread.ticket.id));
      await load();
    } catch (err) {
      setThreadError(err instanceof Error ? err.message : "Reply failed");
    } finally {
      setBusy(false);
    }
  };

  const setTicketStatus = async (status: string) => {
    if (!thread) return;
    setBusy(true);
    try {
      await adminUpdateTicket(thread.ticket.id, { status });
      setThread(await adminTicketDetail(thread.ticket.id));
      await load();
    } catch (err) {
      setThreadError(err instanceof Error ? err.message : "Update failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <AdminShell active="support" title="Support queue" subtitle="Internal notes are never visible to customers. Replies move tickets back to the customer.">
      {error ? <p role="alert" className="rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}
      {threadError ? <p role="alert" className="rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{threadError}</p> : null}
      <div className="flex gap-2">
        <select
          value={statusFilter}
          onChange={(e) => setStatusFilter(e.target.value)}
          aria-label="Status filter"
          className="h-10 rounded-lg border border-ink-200 bg-white px-3 text-sm"
        >
          <option value="">Any status</option>
          {["open", "assigned", "waiting_for_customer", "resolved", "closed"].map((s) => (
            <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
          ))}
        </select>
      </div>

      <div className="mt-4 grid gap-4 lg:grid-cols-2">
        <div>
          {!tickets ? (
            <Skeleton className="h-64 w-full" />
          ) : tickets.length === 0 ? (
            <EmptyState icon="🎧" title="Queue empty" />
          ) : (
            <ul className="space-y-2">
              {tickets.map((t) => (
                <li key={t.id}>
                  <button
                    onClick={() => openThread(t.id)}
                    className={cx(
                      "w-full rounded-xl border px-4 py-3 text-left text-sm shadow-card transition-colors",
                      thread?.ticket.id === t.id ? "border-coral-500 bg-coral-50" : "border-ink-100 bg-white hover:bg-cream-50",
                    )}
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="font-bold">{t.reference}</span>
                      <Badge tone={t.status === "resolved" || t.status === "closed" ? "leaf" : t.priority === "urgent" || t.priority === "high" ? "berry" : "neutral"}>
                        {t.status}
                      </Badge>
                    </div>
                    <p className="mt-1 truncate">{t.subject}</p>
                    <p className="mt-0.5 text-xs text-ink-500">
                      {t.category} · {t.customer_email} · {t.priority} priority
                    </p>
                  </button>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div>
          {!thread ? (
            <EmptyState icon="💬" title="Select a ticket" description="Choose a ticket from the queue to read the thread and reply." />
          ) : (
            <Card>
              <CardBody>
                <div className="flex items-center justify-between">
                  <span className="font-extrabold">{thread.ticket.reference}</span>
                  <Badge tone="neutral">{thread.ticket.status}</Badge>
                </div>
                <p className="mt-1 text-sm">{thread.ticket.subject}</p>
                {thread.ticket.order_id ? (
                  <p className="mt-1 text-xs text-ink-500">
                    Order-linked: {thread.ticket.order_id}
                  </p>
                ) : null}
                <ul className="mt-3 space-y-2">
                  {thread.messages.map((m) => (
                    <li
                      key={m.id}
                      className={cx(
                        "rounded-lg px-3 py-2 text-sm",
                        m.is_internal_note
                          ? "bg-marigold-50 border border-marigold-100"
                          : m.author_type === "agent"
                            ? "bg-sky-50"
                            : "bg-cream-100",
                      )}
                    >
                      <p className="text-[11px] font-bold uppercase text-ink-500">
                        {m.author_type}
                        {m.is_internal_note ? " · INTERNAL — never shown to customer" : ""}
                      </p>
                      <p className="mt-1">{m.body}</p>
                    </li>
                  ))}
                </ul>
                <div className="mt-4">
                  <textarea
                    value={reply}
                    onChange={(e) => setReply(e.target.value)}
                    rows={3}
                    aria-label="Reply"
                    placeholder="Write a reply (internal notes stay hidden from customers)…"
                    className="w-full rounded-lg border border-ink-200 px-3 py-2 text-sm"
                  />
                  <label className="mt-2 flex items-center gap-2 text-xs font-semibold text-ink-700">
                    <input type="checkbox" checked={internal} onChange={(e) => setInternal(e.target.checked)} />
                    Internal note
                  </label>
                  <div className="mt-2 flex flex-wrap gap-2">
                    <Button size="sm" onClick={sendReply} loading={busy} disabled={reply.trim().length < 2}>
                      Send
                    </Button>
                    <Select
                      value=""
                      aria-label="Set status"
                      onChange={(e) => e.target.value && setTicketStatus(e.target.value)}
                      className="w-44"
                    >
                      <option value="">Set status…</option>
                      {["assigned", "waiting_for_customer", "resolved", "closed"].map((s) => (
                        <option key={s} value={s}>{s.replace(/_/g, " ")}</option>
                      ))}
                    </Select>
                  </div>
                </div>
              </CardBody>
            </Card>
          )}
        </div>
      </div>
    </AdminShell>
  );
}

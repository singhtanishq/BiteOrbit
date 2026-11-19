"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import {
  createSupportTicket,
  friendlyError,
  mySupportTickets,
  replySupportTicket,
  SupportTicketRow,
  supportThread,
  SupportThread,
} from "@biteorbit/api-client";
import { Badge, Button, Card, CardBody, CardHeader, CardTitle, EmptyState, Input, Select, Skeleton, Textarea, cx } from "@biteorbit/ui";
import { useSession } from "@/lib/useSession";

const CATEGORIES = [
  ["missing_item", "Missing item"],
  ["wrong_item", "Wrong item delivered"],
  ["quality", "Food quality"],
  ["late_delivery", "Late delivery"],
  ["payment", "Payment issue"],
  ["cancellation_refund", "Cancellation / refund"],
  ["conduct", "Rider or restaurant conduct"],
  ["account", "Account"],
  ["other", "Something else"],
] as const;

export default function SupportPage() {
  const { user, loaded } = useSession();
  const [tickets, setTickets] = useState<SupportTicketRow[] | null>(null);
  const [thread, setThread] = useState<SupportThread | null>(null);
  const [creating, setCreating] = useState(false);
  const [category, setCategory] = useState("late_delivery");
  const [subject, setSubject] = useState("");
  const [body, setBody] = useState("");
  const [reply, setReply] = useState("");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);

  const load = useCallback(async () => {
    try {
      setTickets(await mySupportTickets());
    } catch (err) {
      setError(friendlyError(err));
      setTickets([]);
    }
  }, []);

  useEffect(() => {
    if (loaded) load();
  }, [loaded, load]);

  const create = async (e: React.FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await createSupportTicket(category, subject.trim(), body.trim());
      setNotice(`${res.reference}: ${res.message}`);
      setCreating(false);
      setSubject("");
      setBody("");
      await load();
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const sendReply = async () => {
    if (!thread || reply.trim().length < 2) return;
    setBusy(true);
    try {
      await replySupportTicket(thread.ticket.id, reply.trim());
      setReply("");
      setThread(await supportThread(thread.ticket.id));
      await load();
    } catch (err) {
      setError(friendlyError(err));
    } finally {
      setBusy(false);
    }
  };

  const openThreadId = thread?.ticket.id ?? null;

  if (loaded && !user) {
    return (
      <main className="mx-auto max-w-2xl px-4 py-10">
        <h1 className="text-2xl font-extrabold">Help & support</h1>
        <EmptyState
          icon="🎧"
          title="Sign in to contact support"
          description="Tickets are tied to your demo account so agents can see your orders."
          className="mt-6"
          action={<Link href="/account"><Button>Sign in</Button></Link>}
        />
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-extrabold">Help & support</h1>
        <Button size="sm" onClick={() => setCreating((v) => !v)}>
          {creating ? "Cancel" : "New ticket"}
        </Button>
      </div>
      {notice ? <p role="status" className="mt-4 rounded-lg bg-leaf-50 px-4 py-3 text-sm text-leaf-700">{notice}</p> : null}
      {error ? <p role="alert" className="mt-4 rounded-lg bg-berry-50 px-4 py-3 text-sm text-berry-700">{error}</p> : null}

      {creating ? (
        <Card className="mt-4">
          <CardHeader><CardTitle>Open a ticket</CardTitle></CardHeader>
          <CardBody>
            <form onSubmit={create} className="space-y-3" noValidate>
              <Select label="Category" value={category} onChange={(e) => setCategory(e.target.value)}>
                {CATEGORIES.map(([v, l]) => <option key={v} value={v}>{l}</option>)}
              </Select>
              <Input label="Subject" value={subject} onChange={(e) => setSubject(e.target.value)} required maxLength={150} />
              <Textarea label="What happened?" value={body} onChange={(e) => setBody(e.target.value)} required rows={4} maxLength={4000} />
              <Button type="submit" loading={busy} disabled={subject.trim().length < 4 || body.trim().length < 10}>
                Submit ticket
              </Button>
            </form>
          </CardBody>
        </Card>
      ) : null}

      {thread ? (
        <Card className="mt-4">
          <CardHeader>
            <div className="flex items-center justify-between">
              <CardTitle>{thread.ticket.reference}</CardTitle>
              <Badge tone="neutral">{thread.ticket.status}</Badge>
            </div>
          </CardHeader>
          <CardBody>
            <ul className="space-y-2">
              {thread.messages.map((m) => (
                <li
                  key={m.id}
                  className={cx(
                    "rounded-lg px-3 py-2 text-sm",
                    m.author_type === "agent" ? "bg-sky-50" : "bg-cream-100",
                  )}
                >
                  <p className="text-[11px] font-bold uppercase text-ink-500">{m.author_type}</p>
                  <p className="mt-1">{m.body}</p>
                </li>
              ))}
            </ul>
            <div className="mt-3">
              <Textarea
                label="Reply"
                value={reply}
                onChange={(e) => setReply(e.target.value)}
                rows={2}
                maxLength={4000}
              />
              <Button size="sm" className="mt-2" onClick={sendReply} loading={busy} disabled={reply.trim().length < 2}>
                Send reply
              </Button>
            </div>
            <button className="mt-3 text-xs font-bold text-coral-600" onClick={() => setThread(null)}>
              ← Back to tickets
            </button>
          </CardBody>
        </Card>
      ) : !tickets ? (
        <Skeleton className="mt-4 h-40 w-full" />
      ) : tickets.length === 0 && !creating ? (
        <EmptyState
          icon="🎧"
          title="No tickets"
          description="Order issues, refunds and feedback all live here."
          className="mt-4"
        />
      ) : (
        <ul className="mt-4 space-y-2">
          {tickets?.map((t) => (
            <li key={t.id}>
              <button
                onClick={async () => {
                  try {
                    setThread(await supportThread(t.id));
                  } catch (err) {
                    setError(friendlyError(err));
                  }
                }}
                className={cx(
                  "w-full rounded-xl border px-4 py-3 text-left text-sm shadow-card",
                  openThreadId === t.id ? "border-coral-500 bg-coral-50" : "border-ink-100 bg-white hover:bg-cream-50",
                )}
              >
                <div className="flex items-center justify-between">
                  <span className="font-bold">{t.reference}</span>
                  <Badge tone={t.status === "resolved" || t.status === "closed" ? "leaf" : "neutral"}>
                    {t.status.replace(/_/g, " ")}
                  </Badge>
                </div>
                <p className="mt-1">{t.subject}</p>
                <p className="mt-0.5 text-xs text-ink-500">{t.replies} message(s) · updated {new Date(t.updated_at).toLocaleDateString("en-IN")}</p>
              </button>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
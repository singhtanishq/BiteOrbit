"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  friendlyError,
  listNotifications,
  markAllNotificationsRead,
  markNotificationRead,
  AppNotification,
} from "@biteorbit/api-client";
import { cx } from "@biteorbit/ui";

/** Header notification bell with unread count and dropdown. */
export function NotificationsBell() {
  const [open, setOpen] = useState(false);
  const [items, setItems] = useState<AppNotification[] | null>(null);
  const [unread, setUnread] = useState(0);
  const [error, setError] = useState("");
  const rootRef = useRef<HTMLDivElement>(null);

  const load = useCallback(async () => {
    try {
      const rows = await listNotifications();
      setItems(rows);
      setUnread(rows.filter((r) => !r.read_at).length);
    } catch {
      setItems([]);
      setUnread(0);
    }
  }, []);

  useEffect(() => {
    load();
    const timer = setInterval(load, 20_000);
    return () => clearInterval(timer);
  }, [load]);

  useEffect(() => {
    const onClick = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  const markRead = async (id: string) => {
    try {
      await markNotificationRead(id);
      await load();
    } catch (err) {
      setError(friendlyError(err));
    }
  };

  return (
    <div ref={rootRef} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        aria-label={`Notifications, ${unread} unread`}
        className="relative rounded-full p-2 text-ink-600 hover:bg-cream-200"
      >
        <svg width="18" height="18" viewBox="0 0 24 24" fill="none" aria-hidden="true">
          <path
            d="M12 3a6 6 0 0 0-6 6v3.3l-1.4 3A1 1 0 0 0 5.5 17h13a1 1 0 0 0 .9-1.6L18 12.3V9a6 6 0 0 0-6-6Z"
            stroke="currentColor"
            strokeWidth="1.8"
          />
          <path d="M10 19a2 2 0 0 0 4 0" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" />
        </svg>
        {unread > 0 ? (
          <span className="absolute -right-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-coral-500 px-1 text-[10px] font-bold text-white">
            {unread}
          </span>
        ) : null}
      </button>
      {open ? (
        <div className="absolute right-0 z-50 mt-2 w-80 rounded-xl border border-ink-200 bg-white shadow-pop">
          <div className="flex items-center justify-between border-b border-ink-100 px-4 py-2.5">
            <span className="text-sm font-bold">Notifications</span>
            <button
              className="text-xs font-semibold text-coral-600 hover:underline"
              onClick={async () => {
                try {
                  await markAllNotificationsRead();
                  await load();
                } catch {
                  /* best effort */
                }
              }}
            >
              Mark all read
            </button>
          </div>
          {error ? <p className="px-4 py-2 text-xs text-berry-600">{error}</p> : null}
          <ul className="max-h-80 overflow-y-auto">
            {items === null ? (
              <li className="px-4 py-6 text-center text-xs text-ink-400">Loading…</li>
            ) : items.length === 0 ? (
              <li className="px-4 py-6 text-center text-xs text-ink-400">
                Nothing yet — order updates land here.
              </li>
            ) : (
              items.map((n) => (
                <li key={n.id}>
                  <button
                    onClick={() => {
                      if (!n.read_at) markRead(n.id);
                    }}
                    className={cx(
                      "block w-full border-b border-ink-50 px-4 py-2.5 text-left text-sm hover:bg-cream-50",
                      !n.read_at && "bg-coral-50/50",
                    )}
                  >
                    <span className="flex items-center gap-2">
                      {!n.read_at ? <span className="h-2 w-2 rounded-full bg-coral-500" aria-hidden="true" /> : null}
                      <span className="font-bold text-ink-900">{n.title}</span>
                    </span>
                    <span className="mt-0.5 block text-xs text-ink-600">{n.body}</span>
                    <span className="mt-0.5 block text-[10px] text-ink-400">
                      {new Date(n.created_at).toLocaleString("en-IN")}
                    </span>
                  </button>
                </li>
              ))
            )}
          </ul>
        </div>
      ) : null}
    </div>
  );
}

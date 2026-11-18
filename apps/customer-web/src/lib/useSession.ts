"use client";

import { useEffect, useState } from "react";
import { getSession, SessionUser } from "@biteorbit/api-client";

/** Lightweight session hook for header state (signed-in? name? roles?). */
export function useSession() {
  const [user, setUser] = useState<SessionUser | null>(null);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    let alive = true;
    getSession()
      .then((u) => {
        if (alive) setUser(u);
      })
      .catch(() => {
        if (alive) setUser(null);
      })
      .finally(() => {
        if (alive) setLoaded(true);
      });
    return () => {
      alive = false;
    };
  }, []);

  return { user, loaded };
}

"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";

export default function PortalHome() {
  const router = useRouter();
  useEffect(() => {
    router.replace("/dashboard");
  }, [router]);
  return (
    <main className="mx-auto max-w-md px-4 py-16 text-center">
      <p className="text-sm text-ink-500">Loading partner console…</p>
    </main>
  );
}

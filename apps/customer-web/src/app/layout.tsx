import type { Metadata, Viewport } from "next";
import Link from "next/link";
import { DemoBanner, Logo } from "@biteorbit/ui";
import "./globals.css";

export const metadata: Metadata = {
  title: "BiteOrbit — Great food. Right to your door.",
  description:
    "BiteOrbit is a food discovery and delivery marketplace. This local build uses synthetic demo data only.",
};

export const viewport: Viewport = {
  width: "device-width",
  initialScale: 1,
  themeColor: "#f95d33",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body className="min-h-dvh bg-cream-50">
        <DemoBanner />
        <header className="sticky top-0 z-40 border-b border-ink-100 bg-cream-50/95 backdrop-blur">
          <div className="mx-auto flex h-14 max-w-6xl items-center justify-between gap-4 px-4">
            <Link href="/" aria-label="BiteOrbit home" className="flex items-center">
              <Logo />
            </Link>
            <nav aria-label="Primary" className="flex items-center gap-1 text-sm font-semibold">
              <Link className="rounded-full px-3 py-1.5 hover:bg-cream-200" href="/">
                Discover
              </Link>
              <Link className="rounded-full px-3 py-1.5 hover:bg-cream-200" href="/orders">
                Orders
              </Link>
              <Link className="rounded-full px-3 py-1.5 hover:bg-cream-200" href="/account">
                Account
              </Link>
              <Link
                className="ml-2 rounded-full bg-coral-500 px-4 py-1.5 text-white hover:bg-coral-600"
                href="/cart"
              >
                Cart
              </Link>
            </nav>
          </div>
        </header>
        {children}
        <footer className="mt-16 border-t border-ink-100 py-8 text-center text-xs text-ink-500">
          <p>
            BiteOrbit — a local demo build. Synthetic data, simulated payments, no real fulfilment.
          </p>
        </footer>
      </body>
    </html>
  );
}

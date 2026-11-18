#!/usr/bin/env node
/**
 * Derives the restaurant / courier / admin portal shells from the canonical
 * customer-web app, adapting package identity, port, nav and placeholder page.
 * Run once from the repo root: `node scripts/scaffold-portal.mjs`.
 */
import { cpSync, readFileSync, writeFileSync, existsSync, rmSync } from "node:fs";
import { join } from "node:path";

const root = process.cwd();
const source = "customer-web";

const portals = [
  {
    dir: "restaurant-web",
    pkg: "@biteorbit/restaurant-web",
    port: 3001,
    title: "BiteOrbit Partner — restaurant console",
    nav: [
      { href: "/dashboard", label: "Dashboard" },
      { href: "/orders", label: "Orders" },
      { href: "/menu", label: "Menu" },
      { href: "/settings", label: "Settings" },
    ],
    home: { heading: "Restaurant console shell", body: "Order queue and menu management arrive with Milestone 08." },
  },
  {
    dir: "courier-web",
    pkg: "@biteorbit/courier-web",
    port: 3002,
    title: "BiteOrbit Partner — delivery console",
    nav: [
      { href: "/dashboard", label: "Today" },
      { href: "/offers", label: "Offers" },
      { href: "/earnings", label: "Earnings" },
      { href: "/profile", label: "Profile" },
    ],
    home: { heading: "Delivery partner console shell", body: "Dispatch offers and delivery lifecycle arrive with Milestone 09." },
  },
  {
    dir: "admin-web",
    pkg: "@biteorbit/admin-web",
    port: 3003,
    title: "BiteOrbit Admin — operations console",
    nav: [
      { href: "/overview", label: "Overview" },
      { href: "/orders", label: "Orders" },
      { href: "/restaurants", label: "Restaurants" },
      { href: "/users", label: "Users" },
    ],
    home: { heading: "Platform administration shell", body: "Operational dashboards and management modules arrive with Milestone 10." },
  },
];

for (const portal of portals) {
  const target = join(root, "apps", portal.dir);
  cpSync(join(root, "apps", source), target, {
    recursive: true,
    filter: (src) => !src.includes("node_modules") && !src.includes(".next"),
  });

  // package.json identity
  const pkgPath = join(target, "package.json");
  const pkg = JSON.parse(readFileSync(pkgPath, "utf8"));
  pkg.name = portal.pkg;
  pkg.scripts.dev = pkg.scripts.dev.replace("-p 3000", `-p ${portal.port}`);
  pkg.scripts.start = pkg.scripts.start.replace("-p 3000", `-p ${portal.port}`);
  writeFileSync(pkgPath, JSON.stringify(pkg, null, 2) + "\n");

  // layout: rebrand nav
  const layoutPath = join(target, "src", "app", "layout.tsx");
  let layout = readFileSync(layoutPath, "utf8");
  const navLinks = portal.nav
    .map(
      (n) =>
        `              <Link className="rounded-full px-3 py-1.5 hover:bg-cream-200" href="${n.href}">\n                ${n.label}\n              </Link>`,
    )
    .join("\n");
  const navBlock = `            <nav aria-label="Primary" className="flex items-center gap-1 text-sm font-semibold">
${navLinks}
            </nav>`;
  layout = layout.replace(
    /            <nav aria-label="Primary"[\s\S]*?<\/nav>/,
    navBlock,
  );
  layout = layout.replace(
    /export const metadata: Metadata = \{[\s\S]*?\n\};/,
    `export const metadata: Metadata = {\n  title: "${portal.title}",\n  description:\n    "BiteOrbit operations console. This local build uses synthetic demo data only.",\n};`,
  );
  writeFileSync(layoutPath, layout);

  // home page placeholder
  const homePath = join(target, "src", "app", "page.tsx");
  const home = `export default function HomePage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-10">
      <section className="rounded-2xl bg-cream-100 px-6 py-12 text-center border border-ink-100">
        <h1 className="text-3xl font-extrabold tracking-tight text-ink-900">
          ${portal.home.heading}
        </h1>
        <p className="mx-auto mt-3 max-w-xl text-sm text-ink-600">
          ${portal.home.body}
        </p>
      </section>
    </main>
  );
}
`;
  writeFileSync(homePath, home);

  // remove customer-specific pages that the portal doesn't use yet
  for (const extra of ["orders", "cart", "account"]) {
    const p = join(target, "src", "app", extra);
    if (existsSync(p)) rmSync(p, { recursive: true });
  }
  console.log(`scaffolded apps/${portal.dir} (port ${portal.port})`);
}

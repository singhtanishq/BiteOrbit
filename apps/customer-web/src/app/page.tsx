import Link from "next/link";
import { Card, CardBody, CardHeader, CardTitle } from "@biteorbit/ui";

const portals = [
  {
    href: "/",
    title: "Customer app",
    body: "Discover restaurants, build a cart, check out and track orders — coming online milestone by milestone.",
    icon: "🛵",
  },
];

export default function HomePage() {
  return (
    <main className="mx-auto max-w-6xl px-4 py-10">
      <section className="rounded-2xl bg-coral-50 px-6 py-12 text-center">
        <h1 className="text-3xl font-extrabold tracking-tight text-ink-900 sm:text-4xl">
          Great food. Great local places.
          <span className="block text-coral-600">Right to your door.</span>
        </h1>
        <p className="mx-auto mt-3 max-w-xl text-sm text-ink-600">
          Discovery, ordering, live tracking and support for the demo city of Bitepur. Restaurant
          discovery arrives with Milestone 04.
        </p>
      </section>
      <section className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {portals.map((p) => (
          <Card key={p.title}>
            <CardHeader>
              <CardTitle>
                <span aria-hidden="true" className="mr-2">
                  {p.icon}
                </span>
                {p.title}
              </CardTitle>
            </CardHeader>
            <CardBody className="text-sm text-ink-600">
              {p.body}
              <div className="mt-3">
                <Link href={p.href} className="font-semibold text-coral-600 hover:underline">
                  Open →
                </Link>
              </div>
            </CardBody>
          </Card>
        ))}
        <Card className="opacity-70">
          <CardHeader>
            <CardTitle>🔧 Platform build status</CardTitle>
          </CardHeader>
          <CardBody className="text-sm text-ink-600">
            This shell is part of Milestone 01. Backend, catalogue and checkout land in subsequent
            milestones — each verified and committed independently.
          </CardBody>
        </Card>
      </section>
    </main>
  );
}

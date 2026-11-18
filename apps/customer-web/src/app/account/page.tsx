import { EmptyState } from "@biteorbit/ui";

export default function AccountPage() {
  return (
    <main className="mx-auto max-w-3xl px-4 py-10">
      <h1 className="text-2xl font-extrabold">Account</h1>
      <EmptyState
        icon="👤"
        title="Sign in coming soon"
        description="Profiles, addresses and saved details arrive with the identity milestone."
        className="mt-6"
      />
    </main>
  );
}

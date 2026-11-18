import { cx } from "../cx";

export function EmptyState({
  icon = "🍽️",
  title,
  description,
  action,
  className,
}: {
  icon?: string;
  title: string;
  description?: string;
  action?: React.ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cx(
        "flex flex-col items-center justify-center rounded-xl border border-dashed border-ink-200 bg-cream-100/60 px-6 py-12 text-center",
        className,
      )}
    >
      <span aria-hidden="true" className="text-4xl">
        {icon}
      </span>
      <h3 className="mt-3 text-base font-bold text-ink-900">{title}</h3>
      {description ? <p className="mt-1 max-w-sm text-sm text-ink-600">{description}</p> : null}
      {action ? <div className="mt-4">{action}</div> : null}
    </div>
  );
}

/** Layout-preserving skeleton block for loading states. */
export function Skeleton({ className }: { className?: string }) {
  return (
    <div aria-hidden="true" className={cx("animate-pulse rounded-md bg-ink-100", className)} />
  );
}

export function Spinner({ className, label = "Loading" }: { className?: string; label?: string }) {
  return (
    <span role="status" aria-label={label} className={cx("inline-flex items-center", className)}>
      <span className="h-5 w-5 animate-spin rounded-full border-2 border-coral-500 border-t-transparent" />
      <span className="sr-only">{label}…</span>
    </span>
  );
}

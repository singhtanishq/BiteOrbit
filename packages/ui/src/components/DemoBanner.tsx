import { cx } from "../cx";

/**
 * Persistent, unmistakable notice that the environment is a local demo with
 * synthetic data and simulated integrations (payments, SMS, maps, email).
 */
export function DemoBanner({
  message = "Local demo — all restaurants, people and payments on this site are synthetic and simulated. No real orders are fulfilled and no real money moves.",
  className,
  compact = false,
}: {
  message?: string;
  className?: string;
  compact?: boolean;
}) {
  return (
    <div
      role="note"
      aria-label="Demo mode notice"
      className={cx(
        "w-full bg-ink-900 text-cream-100",
        compact ? "px-3 py-1 text-[11px]" : "px-4 py-1.5 text-xs",
        className,
      )}
    >
      <span className="mx-auto flex max-w-6xl items-center gap-2">
        <span className="rounded-full bg-marigold-500 px-1.5 py-px text-[10px] font-bold text-ink-900">
          DEMO
        </span>
        <span className="opacity-90">{message}</span>
      </span>
    </div>
  );
}

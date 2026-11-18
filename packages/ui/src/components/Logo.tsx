import { cx } from "../cx";

/**
 * Original BiteOrbit mark: a plate (circle) with an orbiting bite-satellite.
 * Pure inline SVG — no external asset dependencies.
 */
export function Logo({
  size = 32,
  withWordmark = true,
  className,
  tone = "dark",
}: {
  size?: number;
  withWordmark?: boolean;
  className?: string;
  tone?: "dark" | "light";
}) {
  const inkColor = tone === "dark" ? "text-ink-900" : "text-cream-50";
  return (
    <span className={cx("inline-flex items-center gap-2", className)} aria-label="BiteOrbit">
      <svg
        width={size}
        height={size}
        viewBox="0 0 48 48"
        fill="none"
        aria-hidden="true"
        className="shrink-0"
      >
        <circle cx="24" cy="24" r="21" fill="var(--color-coral-500)" />
        <circle cx="24" cy="26" r="11" fill="var(--color-cream-50)" />
        {/* bite taken out of the plate */}
        <circle cx="31" cy="21" r="5.5" fill="var(--color-coral-500)" />
        {/* orbit ring + satellite */}
        <ellipse
          cx="24"
          cy="26"
          rx="19"
          ry="7"
          stroke="var(--color-cream-50)"
          strokeWidth="2"
          transform="rotate(-18 24 26)"
          opacity="0.9"
        />
        <circle cx="40.5" cy="17.5" r="3.5" fill="var(--color-marigold-500)" />
        {/* steam */}
        <path
          d="M20 21c0-2 1.6-2.4 1.6-4.2M25 20c0-2 1.6-2.4 1.6-4.2"
          stroke="var(--color-ink-800)"
          strokeWidth="1.8"
          strokeLinecap="round"
          opacity="0.75"
        />
      </svg>
      {withWordmark ? (
        <span className={cx("font-extrabold tracking-tight text-[1.05rem] leading-none", inkColor)}>
          Bite<span className="text-coral-500">Orbit</span>
        </span>
      ) : null}
    </span>
  );
}

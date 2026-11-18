import { cx } from "../cx";

export type BadgeTone = "neutral" | "coral" | "leaf" | "marigold" | "berry" | "sky";

const toneClasses: Record<BadgeTone, string> = {
  neutral: "bg-cream-200 text-ink-700 border-ink-200",
  coral: "bg-coral-50 text-coral-700 border-coral-200",
  leaf: "bg-leaf-50 text-leaf-700 border-leaf-100",
  marigold: "bg-marigold-50 text-marigold-600 border-marigold-100",
  berry: "bg-berry-50 text-berry-700 border-berry-100",
  sky: "bg-sky-50 text-sky-600 border-sky-100",
};

export function Badge({
  tone = "neutral",
  className,
  children,
}: {
  tone?: BadgeTone;
  className?: string;
  children: React.ReactNode;
}) {
  return (
    <span
      className={cx(
        "inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-semibold",
        toneClasses[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

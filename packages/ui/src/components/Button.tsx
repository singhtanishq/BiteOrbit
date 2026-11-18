"use client";

import { forwardRef } from "react";
import { cx } from "../cx";

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger" | "success";
type ButtonSize = "sm" | "md" | "lg";

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
}

const variantClasses: Record<ButtonVariant, string> = {
  primary: "bg-coral-500 text-white hover:bg-coral-600 active:bg-coral-700 disabled:bg-coral-200",
  secondary:
    "bg-cream-100 text-ink-900 border border-ink-200 hover:bg-cream-200 active:bg-cream-300 disabled:opacity-50",
  ghost: "bg-transparent text-ink-700 hover:bg-cream-200 disabled:opacity-50",
  danger: "bg-berry-500 text-white hover:bg-berry-600 disabled:bg-berry-100",
  success: "bg-leaf-600 text-white hover:bg-leaf-700 disabled:bg-leaf-100",
};

const sizeClasses: Record<ButtonSize, string> = {
  sm: "h-8 px-3 text-xs gap-1.5",
  md: "h-10 px-4 text-sm gap-2",
  lg: "h-12 px-6 text-base gap-2",
};

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(function Button(
  { variant = "primary", size = "md", loading = false, className, children, disabled, ...rest },
  ref,
) {
  return (
    <button
      ref={ref}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={cx(
        "inline-flex items-center justify-center rounded-md font-semibold transition-colors select-none",
        "focus-visible:outline-2 focus-visible:outline-coral-500 focus-visible:outline-offset-2",
        "disabled:cursor-not-allowed",
        variantClasses[variant],
        sizeClasses[size],
        className,
      )}
      {...rest}
    >
      {loading ? (
        <span
          aria-hidden="true"
          className="h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent"
        />
      ) : null}
      {children}
    </button>
  );
});

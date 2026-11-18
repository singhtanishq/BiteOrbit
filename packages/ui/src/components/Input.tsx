"use client";

import { forwardRef, useId } from "react";
import { cx } from "../cx";

export interface InputProps extends React.InputHTMLAttributes<HTMLInputElement> {
  label?: string;
  error?: string;
  hint?: string;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(function Input(
  { label, error, hint, className, id, ...rest },
  ref,
) {
  const autoId = useId();
  const inputId = id ?? autoId;
  const errorId = error ? `${inputId}-error` : undefined;
  const hintId = hint ? `${inputId}-hint` : undefined;
  return (
    <div className="w-full">
      {label ? (
        <label htmlFor={inputId} className="mb-1.5 block text-sm font-semibold text-ink-800">
          {label}
        </label>
      ) : null}
      <input
        ref={ref}
        id={inputId}
        aria-invalid={error ? true : undefined}
        aria-describedby={errorId ?? hintId}
        className={cx(
          "h-10 w-full rounded-md border bg-white px-3 text-sm text-ink-900 placeholder:text-ink-400",
          "focus:outline-2 focus:outline-offset-0 focus:outline-coral-500",
          error ? "border-berry-500" : "border-ink-200 focus:border-coral-500",
          className,
        )}
        {...rest}
      />
      {hint && !error ? (
        <p id={hintId} className="mt-1 text-xs text-ink-500">
          {hint}
        </p>
      ) : null}
      {error ? (
        <p id={errorId} className="mt-1 text-xs font-medium text-berry-600" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
});

export interface SelectProps extends React.SelectHTMLAttributes<HTMLSelectElement> {
  label?: string;
  error?: string;
  children: React.ReactNode;
}

export const Select = forwardRef<HTMLSelectElement, SelectProps>(function Select(
  { label, error, className, children, id, ...rest },
  ref,
) {
  const autoId = useId();
  const selectId = id ?? autoId;
  return (
    <div className="w-full">
      {label ? (
        <label htmlFor={selectId} className="mb-1.5 block text-sm font-semibold text-ink-800">
          {label}
        </label>
      ) : null}
      <select
        ref={ref}
        id={selectId}
        className={cx(
          "h-10 w-full rounded-md border bg-white px-3 text-sm text-ink-900",
          "focus:outline-2 focus:outline-offset-0 focus:outline-coral-500",
          error ? "border-berry-500" : "border-ink-200",
          className,
        )}
        {...rest}
      >
        {children}
      </select>
      {error ? (
        <p className="mt-1 text-xs font-medium text-berry-600" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
});

export interface TextareaProps extends React.TextareaHTMLAttributes<HTMLTextAreaElement> {
  label?: string;
  error?: string;
}

export const Textarea = forwardRef<HTMLTextAreaElement, TextareaProps>(function Textarea(
  { label, error, className, id, ...rest },
  ref,
) {
  const autoId = useId();
  const areaId = id ?? autoId;
  return (
    <div className="w-full">
      {label ? (
        <label htmlFor={areaId} className="mb-1.5 block text-sm font-semibold text-ink-800">
          {label}
        </label>
      ) : null}
      <textarea
        ref={ref}
        id={areaId}
        aria-invalid={error ? true : undefined}
        className={cx(
          "w-full rounded-md border bg-white px-3 py-2 text-sm text-ink-900 placeholder:text-ink-400",
          "focus:outline-2 focus:outline-offset-0 focus:outline-coral-500",
          error ? "border-berry-500" : "border-ink-200",
          className,
        )}
        {...rest}
      />
      {error ? (
        <p className="mt-1 text-xs font-medium text-berry-600" role="alert">
          {error}
        </p>
      ) : null}
    </div>
  );
});

"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { friendlyError, suggestions, Suggestions } from "@biteorbit/api-client";
import { cx } from "@biteorbit/ui";

/**
 * Debounced search with keyboard-navigable suggestions.
 * Suggestions come from the backend (restaurant/cuisine/dish names).
 */
export function SearchBar({
  initialQuery = "",
  placeholder = "Search restaurants, cuisines or dishes…",
  className,
}: {
  initialQuery?: string;
  placeholder?: string;
  className?: string;
}) {
  const router = useRouter();
  const [value, setValue] = useState(initialQuery);
  const [data, setData] = useState<Suggestions | null>(null);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const [error, setError] = useState("");
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (value.trim().length < 2) {
      setData(null);
      return;
    }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(async () => {
      try {
        const res = await suggestions(value.trim());
        setData(res);
        setOpen(true);
        setActive(-1);
        setError("");
      } catch (err) {
        setError(friendlyError(err));
      }
    }, 250);
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, [value]);

  useEffect(() => {
    const onClickOutside = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, []);

  const flat = data
    ? [
        ...data.restaurants.map((r) => ({ kind: "restaurant" as const, id: r.id, label: r.name, sub: r.locality })),
        ...data.cuisines.map((c) => ({ kind: "cuisine" as const, id: c.slug, label: c.name, sub: "cuisine" })),
        ...data.dishes.map((d) => ({ kind: "dish" as const, id: d.id, label: d.name, sub: d.outlet })),
      ]
    : [];

  const submit = (q: string) => {
    const trimmed = q.trim();
    setOpen(false);
    router.push(trimmed ? `/?query=${encodeURIComponent(trimmed)}` : "/");
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!open || flat.length === 0) return;
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActive((a) => Math.min(a + 1, flat.length - 1));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActive((a) => Math.max(a - 1, 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      const chosen = active >= 0 ? flat[active] : undefined;
      if (chosen) {
        pick(chosen);
      } else {
        submit(value);
      }
    } else if (e.key === "Escape") {
      setOpen(false);
    }
  };

  const pick = (entry: (typeof flat)[number]) => {
    setOpen(false);
    if (entry.kind === "restaurant") router.push(`/restaurants/${entry.id}`);
    else if (entry.kind === "cuisine") router.push(`/?cuisine=${encodeURIComponent(entry.id)}`);
    else router.push(`/?query=${encodeURIComponent(entry.label)}`);
  };

  return (
    <div ref={rootRef} className={cx("relative w-full", className)}>
      <form
        role="search"
        onSubmit={(e) => {
          e.preventDefault();
          submit(value);
        }}
      >
        <input
          type="search"
          value={value}
          onChange={(e) => setValue(e.target.value)}
          onFocus={() => flat.length > 0 && setOpen(true)}
          onKeyDown={onKeyDown}
          placeholder={placeholder}
          aria-label="Search food and restaurants"
          aria-expanded={open}
          aria-controls="search-suggestions"
          role="combobox"
          aria-autocomplete="list"
          className="h-11 w-full rounded-lg border border-ink-200 bg-white px-4 pr-10 text-sm shadow-card placeholder:text-ink-400 focus:border-coral-500 focus:outline-2 focus:outline-coral-500"
        />
        <button
          type="submit"
          aria-label="Search"
          className="absolute right-3 top-1/2 -translate-y-1/2 text-ink-500 hover:text-coral-600"
        >
          <svg width="18" height="18" viewBox="0 0 20 20" fill="none" aria-hidden="true">
            <circle cx="9" cy="9" r="6" stroke="currentColor" strokeWidth="2" />
            <path d="M13.5 13.5 18 18" stroke="currentColor" strokeWidth="2" strokeLinecap="round" />
          </svg>
        </button>
      </form>
      {error ? (
        <p role="alert" className="mt-1 text-xs text-berry-600">
          {error}
        </p>
      ) : null}
      {open && flat.length > 0 ? (
        <ul
          id="search-suggestions"
          role="listbox"
          className="absolute z-30 mt-1 w-full overflow-hidden rounded-lg border border-ink-200 bg-white shadow-pop"
        >
          {flat.map((entry, i) => (
            <li key={`${entry.kind}-${entry.id}-${entry.label}`} role="option" aria-selected={i === active}>
              <button
                type="button"
                onMouseEnter={() => setActive(i)}
                onClick={() => pick(entry)}
                className={cx(
                  "flex w-full items-center justify-between px-4 py-2.5 text-left text-sm",
                  i === active ? "bg-cream-100" : "bg-white",
                )}
              >
                <span className="font-medium text-ink-900">{entry.label}</span>
                <span className="ml-3 shrink-0 text-xs text-ink-500">
                  {entry.kind === "restaurant" ? "restaurant" : entry.sub}
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  );
}

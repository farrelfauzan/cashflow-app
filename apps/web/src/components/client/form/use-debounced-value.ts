"use client";

import { useEffect, useState } from "react";

// How long typing pauses before a search goes to the API.
export const SEARCH_DEBOUNCE_MS = 250;

// `value`, but only after it has stopped changing for `delayMs`.
export function useDebouncedValue<T>(
  value: T,
  delayMs: number = SEARCH_DEBOUNCE_MS,
): T {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = setTimeout(() => setDebounced(value), delayMs);
    return () => clearTimeout(timer);
  }, [value, delayMs]);
  return debounced;
}

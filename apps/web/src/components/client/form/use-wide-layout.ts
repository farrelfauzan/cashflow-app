"use client";

import { useSyncExternalStore } from "react";

import { WIDE_LAYOUT_QUERY } from "./utils";

function subscribe(onChange: () => void): () => void {
  const query = window.matchMedia(WIDE_LAYOUT_QUERY);
  query.addEventListener("change", onChange);
  return () => query.removeEventListener("change", onChange);
}

function wideNow(): boolean {
  return window.matchMedia(WIDE_LAYOUT_QUERY).matches;
}

// Unknown on the server and while the page hydrates.
function unknownOnServer(): null {
  return null;
}

// Whether the screen is at least as wide as the iPad layout (Tailwind's `md`): true on iPad and
// desktop, false on a phone, null while the page is rendered on the server or hydrates, where a
// component shows both choices and lets CSS pick. Chosen by width, never by user agent, so a
// touch iPad gets the iPad controls.
export function useWideLayout(): boolean | null {
  return useSyncExternalStore(subscribe, wideNow, unknownOnServer);
}

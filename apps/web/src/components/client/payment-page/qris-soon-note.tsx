"use client";

import { QRIS_SOON_TEXT } from "./utils";

// Until QRIS is built (slice 3). Rendered in one place, so removing it is one line.
export function QrisSoonNote() {
  return (
    <p className="text-sm text-muted-foreground md:text-base">
      {QRIS_SOON_TEXT}
    </p>
  );
}

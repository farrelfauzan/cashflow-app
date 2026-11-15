"use client";

import { EMPTY_TEXT } from "./utils";

// Nothing waits (of the chosen kind): one calm line, no button.
export function CheckQueueEmpty() {
  return (
    <p className="rounded-[14px] border bg-card px-4 py-4 text-base">
      {EMPTY_TEXT}
    </p>
  );
}

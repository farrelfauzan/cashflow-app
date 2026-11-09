import { lazy } from "react";

import { NativeDateInput } from "./native-date-input";

// The calendar popover, in its own chunk: only wide screens render it, so the tenant payment
// page on a phone never downloads the calendar. If the chunk cannot load (a deploy replaced it
// while the tab was open, or the network dropped), the field falls back to the device's own date
// input instead of failing the page, which has no error boundary.
export const LazyDatePicker = lazy(() =>
  import("./date-picker")
    .then((module) => ({ default: module.DatePicker }))
    .catch(() => ({ default: NativeDateInput })),
);

import { lazy } from "react";

// The calendar popover, in its own chunk: only wide screens render it, so the tenant payment
// page on a phone never downloads the calendar.
export const LazyDatePicker = lazy(() =>
  import("./date-picker").then((module) => ({ default: module.DatePicker })),
);

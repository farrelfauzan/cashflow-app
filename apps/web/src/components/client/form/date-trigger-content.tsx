import { CalendarIcon } from "lucide-react";

import { formatFullDate } from "@/lib/format/date";

import { calendarDayToLocalDate, DATE_PLACEHOLDER } from "./utils";

type DateTriggerContentProps = {
  // YYYY-MM-DD, "" for none.
  value: string;
  // The id of the text, so the trigger's accessible name can include the chosen day.
  textId?: string;
};

// What the closed calendar field shows: "15 November 2026" or the placeholder, and the icon.
export function DateTriggerContent({ value, textId }: DateTriggerContentProps) {
  const chosen =
    calendarDayToLocalDate(value) === undefined ? null : formatFullDate(value);
  return (
    <>
      <span id={textId} className="line-clamp-1 min-w-0">
        {chosen ?? DATE_PLACEHOLDER}
      </span>
      <CalendarIcon
        aria-hidden="true"
        className="size-5 text-muted-foreground"
      />
    </>
  );
}

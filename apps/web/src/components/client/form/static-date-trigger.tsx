import { cn } from "@/lib/utils";

import { DateTriggerContent } from "./date-trigger-content";
import { DATE_TRIGGER_CLASS } from "./utils";

type StaticDateTriggerProps = {
  // YYYY-MM-DD, "" for none.
  value: string;
  invalid: boolean;
  disabled: boolean;
  className?: string;
};

// The calendar field's trigger before the calendar code is here (server render, hydration, the
// first load of its chunk): the same box and text, so nothing jumps when the real one replaces
// it. Hidden from assistive technology and the tab order for that moment.
export function StaticDateTrigger({
  value,
  invalid,
  disabled,
  className,
}: StaticDateTriggerProps) {
  return (
    <div
      aria-hidden="true"
      aria-invalid={invalid}
      data-placeholder={value === "" ? "" : undefined}
      data-disabled={disabled ? "" : undefined}
      className={cn(
        DATE_TRIGGER_CLASS,
        "data-[disabled]:cursor-not-allowed data-[disabled]:opacity-50",
        className,
      )}
    >
      <DateTriggerContent value={value} />
    </div>
  );
}

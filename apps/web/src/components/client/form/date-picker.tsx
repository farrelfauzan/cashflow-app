"use client";

import { useState } from "react";
import { id as indonesian } from "react-day-picker/locale";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { formatLongDate, jakartaToday } from "@/lib/format/date";
import { cn } from "@/lib/utils";

import { DateTriggerContent } from "./date-trigger-content";
import type { DateControlProps } from "./type";
import {
  calendarDayLabel,
  calendarDayToLocalDate,
  calendarMonthName,
  calendarWeekdayName,
  DATE_MONTH_LABEL,
  DATE_NEXT_MONTH_LABEL,
  DATE_PICKER_LABEL,
  DATE_PREVIOUS_MONTH_LABEL,
  DATE_TODAY_LABEL,
  DATE_TRIGGER_CLASS,
  DATE_YEAR_LABEL,
  datePickerMonths,
  datePickerOpenMonth,
  isDayWithin,
  localDateToCalendarDay,
} from "./utils";

// The calendar's look: 44px days with 16px numbers, 44px month and year lists, today on mint,
// the chosen day on green, days outside the limits faded (board "iPad · Kolom tanggal dengan
// kalender").
const CALENDAR_CLASS =
  "p-0 [--cell-size:--spacing(11)] [&_button[data-day]]:text-base [&_button[data-day]]:font-normal";
const CALENDAR_CLASS_NAMES = {
  // Rows 2px apart and 8px under the caption, so a six-week month with its footer fits above or
  // below a field halfway down an iPad screen.
  month: "flex w-full flex-col gap-2",
  month_caption: "flex h-11 w-full items-center justify-center px-11",
  dropdowns: "flex h-11 w-full items-center justify-center gap-1.5",
  dropdown_root:
    "relative rounded-lg border-[1.5px] border-input has-focus:border-ring has-focus:ring-[3px] has-focus:ring-ring",
  // The native list covers the whole box, border included, so the touch target is 44px.
  dropdown: "absolute -inset-[1.5px] bg-popover opacity-0",
  caption_label:
    "flex h-[41px] items-center gap-1 rounded-lg pr-1.5 pl-2.5 text-base font-semibold select-none [&>svg]:size-4 [&>svg]:text-muted-foreground",
  weekday: "flex-1 text-sm font-semibold text-muted-foreground select-none",
  week: "mt-0.5 flex w-full",
  today:
    "rounded-md bg-accent font-bold text-accent-foreground data-[selected=true]:rounded-none",
  disabled: "text-muted-foreground opacity-60",
};

// A date field's control on iPad and desktop: a trigger that shows the chosen day and opens the
// shadcn calendar in a popover. Indonesian month and day names, weeks from Monday, today and
// the limits in Jakarta. Picking a day fills the field and closes the popover; Escape closes it
// without a change; both give the focus back to the trigger. Loaded only on wide screens
// (lazy-date-picker.ts), so a phone never downloads the calendar.
export function DatePicker({
  id,
  labelId,
  name,
  controlRef,
  value,
  min,
  max,
  onChange,
  invalid,
  describedBy,
  disabled,
  className,
}: DateControlProps) {
  const [open, setOpen] = useState(false);
  const today = jakartaToday(new Date());
  const textId = `${id}-text`;
  const popupId = `${id}-calendar`;
  const months = datePickerMonths(today, value, min, max);
  const lower = calendarDayToLocalDate(min ?? "");
  const upper = calendarDayToLocalDate(max ?? "");
  const disabledDays = [
    ...(lower === undefined ? [] : [{ before: lower }]),
    ...(upper === undefined ? [] : [{ after: upper }]),
  ];
  const todaySelectable = isDayWithin(today, min, max);

  function choose(day: string) {
    onChange(day);
    setOpen(false);
  }

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <button
          ref={controlRef}
          id={id}
          name={name}
          type="button"
          // A combobox that opens a dialog (the ARIA date picker pattern), which also carries
          // aria-invalid; a plain button does not.
          role="combobox"
          aria-haspopup="dialog"
          aria-expanded={open}
          aria-controls={open ? popupId : undefined}
          aria-labelledby={`${labelId} ${textId}`}
          aria-invalid={invalid}
          aria-describedby={describedBy}
          disabled={disabled}
          data-placeholder={value === "" ? "" : undefined}
          className={cn(DATE_TRIGGER_CLASS, className)}
        >
          <DateTriggerContent value={value} textId={textId} />
        </button>
      </PopoverTrigger>
      <PopoverContent
        id={popupId}
        role="dialog"
        aria-label={DATE_PICKER_LABEL}
        align="start"
        collisionPadding={16}
        // Never past the screen's edge: when neither above nor below the trigger has room for
        // the whole calendar, the days scroll inside and the "Hari ini" row stays in view.
        className="flex max-h-(--radix-popover-content-available-height) w-auto flex-col gap-2 p-3"
        // The calendar focuses the chosen day (or today) itself.
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <div className="min-h-0 overflow-y-auto">
          <Calendar
            mode="single"
            required
            autoFocus
            locale={indonesian}
            weekStartsOn={1}
            showOutsideDays={false}
            captionLayout="dropdown"
            startMonth={months.start}
            endMonth={months.end}
            defaultMonth={datePickerOpenMonth(today, value, min, max)}
            today={calendarDayToLocalDate(today)}
            selected={calendarDayToLocalDate(value)}
            disabled={disabledDays}
            onSelect={(date) => choose(localDateToCalendarDay(date))}
            formatters={{
              formatMonthDropdown: calendarMonthName,
              formatWeekdayName: calendarWeekdayName,
            }}
            labels={{
              labelMonthDropdown: () => DATE_MONTH_LABEL,
              labelYearDropdown: () => DATE_YEAR_LABEL,
              labelPrevious: () => DATE_PREVIOUS_MONTH_LABEL,
              labelNext: () => DATE_NEXT_MONTH_LABEL,
              labelDayButton: (date, modifiers) =>
                calendarDayLabel(date, {
                  today: modifiers.today === true,
                  selected: modifiers.selected === true,
                }),
              labelGrid: (date) =>
                `${calendarMonthName(date)} ${date.getFullYear()}`,
            }}
            className={CALENDAR_CLASS}
            classNames={CALENDAR_CLASS_NAMES}
          />
        </div>
        <div className="flex shrink-0 items-center justify-between gap-3 border-t pt-2">
          <Button
            type="button"
            variant="outline"
            className="min-h-11 text-[15px]"
            disabled={!todaySelectable}
            onClick={() => choose(today)}
          >
            {DATE_TODAY_LABEL}
          </Button>
          <span className="text-sm text-muted-foreground">
            {formatLongDate(today)}
          </span>
        </div>
      </PopoverContent>
    </Popover>
  );
}

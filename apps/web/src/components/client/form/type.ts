import type { RefCallback } from "react";

// What every date control of a DateField takes: the native input on a phone, the calendar
// popover's trigger on iPad and desktop.
export type DateControlProps = {
  // The control's id, which the field's label points at.
  id: string;
  // The label's id: the calendar trigger is named by the label and the chosen day.
  labelId: string;
  // The form's name for the value ("startDate").
  name: string;
  // For moving focus from outside, such as react-hook-form's field ref.
  controlRef: RefCallback<HTMLElement> | undefined;
  // YYYY-MM-DD, "" for none.
  value: string;
  // The first and last day offered (YYYY-MM-DD); the form's schema still checks the value.
  min: string | undefined;
  max: string | undefined;
  onChange: (date: string) => void;
  invalid: boolean;
  // The ids of the error and hint, for aria-describedby; undefined for none.
  describedBy: string | undefined;
  disabled: boolean;
  className?: string;
};

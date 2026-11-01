// The look of the side-panel forms (properties, units, tenants, leases), shared so every form
// reads the same.

// A text field: 52px tall, 17px text on iPad and 16px on phone.
export const INPUT_CLASS =
  "h-13 rounded-xl border-[1.5px] bg-card px-3.5 text-base aria-invalid:border-2 md:text-[17px]";

export const TEXTAREA_CLASS =
  "min-h-28 rounded-xl border-[1.5px] bg-card px-3.5 py-3 text-base aria-invalid:border-2 md:text-[17px]";

// A select's trigger, as tall as a text field.
export const SELECT_TRIGGER_CLASS =
  "w-full rounded-xl border-[1.5px] bg-card px-3.5 text-base aria-invalid:border-2 data-[size=default]:h-13 md:text-[17px]";

// A select's choice, a 44px touch target.
export const SELECT_ITEM_CLASS = "min-h-11 text-base";

export const FIELD_LABEL_CLASS =
  "text-[15px] leading-normal font-semibold md:text-base";

export const FIELD_HINT_CLASS =
  "text-sm leading-[1.45] text-muted-foreground md:text-base";

// The closed trigger of a searchable select: the same box as a select's trigger (a button, since
// the popover's trigger is not Radix's select trigger), 52px tall, with the placeholder muted.
export const SEARCH_SELECT_TRIGGER_CLASS =
  "flex h-13 w-full items-center justify-between gap-2 rounded-xl border-[1.5px] border-input bg-card px-3.5 text-left text-base whitespace-nowrap shadow-xs transition-[color,box-shadow] outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:cursor-not-allowed disabled:opacity-50 aria-invalid:border-2 aria-invalid:border-destructive aria-invalid:ring-destructive/20 data-[placeholder]:text-muted-foreground md:text-[17px] [&_svg]:pointer-events-none [&_svg]:shrink-0";

// The open list: as wide as the trigger, and not taller than a few options, so it scrolls.
export const SEARCH_SELECT_CONTENT_CLASS =
  "w-(--radix-popover-trigger-width) overflow-hidden p-0";

// The list's box takes the height of its content; the search box's wrapper (shadcn's, 36px by
// default) and the box itself are 48px tall, with 16px text.
export const SEARCH_SELECT_COMMAND_CLASS =
  "h-auto **:data-[slot=command-input-wrapper]:h-12";
export const SEARCH_SELECT_INPUT_CLASS = "h-12 text-base";

// A choice of the open list, a 44px touch target with the check at the end.
export const SEARCH_SELECT_ITEM_CLASS =
  "min-h-11 justify-between px-3 py-2 text-base";

// The text of the list when it has no choices or is loading, and the note under it.
export const SEARCH_SELECT_MESSAGE_CLASS =
  "px-3 py-4 text-center text-base text-muted-foreground";

export const SEARCH_SELECT_NOTE_CLASS =
  "border-t px-3 py-3 text-sm leading-[1.45] text-muted-foreground md:text-base";

// What the trigger shows: the option's label, else the label remembered for the value, else
// nothing (the placeholder).
export function searchSelectShownLabel(
  value: string,
  options: readonly { value: string; label: string }[],
  selectedLabel: string | undefined,
): string | undefined {
  if (value === "") return undefined;
  return (
    options.find((option) => option.value === value)?.label ?? selectedLabel
  );
}

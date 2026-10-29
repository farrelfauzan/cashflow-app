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

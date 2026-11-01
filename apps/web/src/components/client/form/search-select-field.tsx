"use client";

import { CheckIcon, ChevronDownIcon } from "lucide-react";
import { useId, useState, type Ref } from "react";

import { FieldError } from "@/components/client/form/field-error";
import {
  Command,
  CommandEmpty,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import { cn } from "@/lib/utils";

import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  SEARCH_SELECT_COMMAND_CLASS,
  SEARCH_SELECT_CONTENT_CLASS,
  SEARCH_SELECT_INPUT_CLASS,
  SEARCH_SELECT_ITEM_CLASS,
  SEARCH_SELECT_MESSAGE_CLASS,
  SEARCH_SELECT_NOTE_CLASS,
  SEARCH_SELECT_TRIGGER_CLASS,
  searchSelectShownLabel,
} from "./utils";

type SearchSelectFieldProps = {
  // For moving focus from outside: a ref object, or a callback such as react-hook-form's.
  triggerRef: Ref<HTMLButtonElement>;
  label: string;
  // "" shows the placeholder.
  value: string;
  // The label to show for `value` while it is not among `options` (the list changed with the
  // search).
  selectedLabel: string | undefined;
  placeholder: string;
  // The choices for the current search; the server filters them, so they are not filtered here.
  options: readonly { value: string; label: string }[];
  onValueChange: (value: string) => void;
  search: string;
  onSearchChange: (search: string) => void;
  searchPlaceholder: string;
  // True while the choices for the current search are not here yet.
  loading: boolean;
  loadingText: string;
  emptyText: string;
  // Under the list, for "there are more, type to search"; null for none.
  listNote: string | null;
  // Under the field; null for none.
  hint: string | null;
  error: string | undefined;
  disabled: boolean;
  className?: string;
};

// A labelled select whose choices are searched on the server, as tall as a text field (like
// SelectField, which it matches while closed), for a list that can outgrow one page. Opens a
// popover with a search box and the choices; picking one closes it and gives the focus back to
// the trigger.
export function SearchSelectField({
  triggerRef,
  label,
  value,
  selectedLabel,
  placeholder,
  options,
  onValueChange,
  search,
  onSearchChange,
  searchPlaceholder,
  loading,
  loadingText,
  emptyText,
  listNote,
  hint,
  error,
  disabled,
  className,
}: SearchSelectFieldProps) {
  const [open, setOpen] = useState(false);
  const id = useId();
  const errorId = `${id}-error`;
  const hintId = `${id}-hint`;
  const listId = `${id}-list`;
  const describedBy = [
    error !== undefined ? errorId : null,
    hint !== null ? hintId : null,
  ]
    .filter((part) => part !== null)
    .join(" ");
  const shown = searchSelectShownLabel(value, options, selectedLabel);

  function handleOpenChange(next: boolean) {
    setOpen(next);
    // The next opening starts from the full list again.
    if (!next) onSearchChange("");
  }

  return (
    <div className={cn("flex min-w-0 flex-col gap-2", className)}>
      <Label htmlFor={`${id}-select`} className={FIELD_LABEL_CLASS}>
        {label}
      </Label>
      <Popover open={open} onOpenChange={handleOpenChange}>
        <PopoverTrigger asChild>
          <button
            ref={triggerRef}
            id={`${id}-select`}
            type="button"
            role="combobox"
            aria-expanded={open}
            aria-controls={listId}
            disabled={disabled}
            aria-invalid={error !== undefined}
            aria-describedby={describedBy === "" ? undefined : describedBy}
            data-placeholder={shown === undefined ? "" : undefined}
            className={SEARCH_SELECT_TRIGGER_CLASS}
          >
            <span className="line-clamp-1 min-w-0">{shown ?? placeholder}</span>
            <ChevronDownIcon aria-hidden="true" className="size-4 opacity-50" />
          </button>
        </PopoverTrigger>
        <PopoverContent align="start" className={SEARCH_SELECT_CONTENT_CLASS}>
          <Command shouldFilter={false} className={SEARCH_SELECT_COMMAND_CLASS}>
            <CommandInput
              value={search}
              onValueChange={onSearchChange}
              placeholder={searchPlaceholder}
              aria-label={searchPlaceholder}
              className={SEARCH_SELECT_INPUT_CLASS}
            />
            <CommandList className="max-h-64">
              {loading ? (
                <p role="status" className={SEARCH_SELECT_MESSAGE_CLASS}>
                  {loadingText}
                </p>
              ) : (
                <>
                  <CommandEmpty className={SEARCH_SELECT_MESSAGE_CLASS}>
                    {emptyText}
                  </CommandEmpty>
                  {options.map((option) => (
                    <CommandItem
                      key={option.value}
                      value={option.value}
                      keywords={[option.label]}
                      onSelect={() => {
                        onValueChange(option.value);
                        handleOpenChange(false);
                      }}
                      className={SEARCH_SELECT_ITEM_CLASS}
                    >
                      {option.label}
                      {option.value === value && (
                        <CheckIcon aria-hidden="true" className="size-4" />
                      )}
                    </CommandItem>
                  ))}
                </>
              )}
            </CommandList>
            {listNote !== null && !loading && (
              <p className={SEARCH_SELECT_NOTE_CLASS}>{listNote}</p>
            )}
          </Command>
        </PopoverContent>
      </Popover>
      {error !== undefined && <FieldError id={errorId} message={error} />}
      {hint !== null && (
        <p id={hintId} className={FIELD_HINT_CLASS}>
          {hint}
        </p>
      )}
    </div>
  );
}

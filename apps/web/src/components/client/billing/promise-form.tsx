"use client";

import { useId } from "react";
import { Controller, useWatch } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import type { FormSheetWideLayout } from "@/components/client/form-sheet/type";
import { ChoiceRadioGroup } from "@/components/client/payment-page/choice-radio-group";
import { PromiseAmountField } from "@/components/client/payment-page/promise-amount-field";
import { PromiseDateField } from "@/components/client/payment-page/promise-date-field";
import {
  offeredPromiseKind,
  PROMISE_KIND_LEGEND,
  PROMISE_SAVING_TEXT,
  toPromiseKind,
} from "@/components/client/payment-page/utils";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import type { MakeTenantPromiseRequest } from "@/lib/api/generated/model";
import { PromiseKind } from "@/lib/api/generated/model";

import type { PromiseDialogView } from "./type";
import {
  PROMISE_NOTE_LABEL,
  PROMISE_NOTE_MAX_LENGTH,
  PROMISE_SHEET_CLASSES,
  STAFF_PROMISE_FORM_FIELDS,
  staffPromiseFormError,
  staffPromiseSchema,
} from "./utils";

import { PromiseSheetHeader } from "./promise-sheet-header";

type PromiseFormProps = {
  // Made when the form opened (from the account, or the field app's offer).
  view: PromiseDialogView;
  // The centred dialog (Detail penyewa) or the drawer (Tagihan hari ini) from md up.
  layout: FormSheetWideLayout;
  saving: boolean;
  // Sends the checked request (makeTenantPromise or makeFieldPromise). A refusal is handed to
  // `onError`, which shows it under its field or in the alert.
  onSave: (
    body: MakeTenantPromiseRequest,
    onError: (error: unknown) => void,
  ) => void;
};

// The fields of a staff payment promise, inside a FormSheetContent: the header, the choice of
// "when", the date, the amount and (on Detail penyewa) the member's note, with Batal and the save
// button pinned at the bottom. Over an active promise it opens filled from it and saving adds a
// version. It never sends payingNow. Its unsaved changes reach the FormSheet through useApiForm.
export function PromiseForm({
  view,
  layout,
  saving,
  onSave,
}: PromiseFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const classes = PROMISE_SHEET_CLASSES[layout];
  const { form, field, showApiError, alert } = useApiForm({
    schema: staffPromiseSchema(view.offer),
    defaultValues: view.defaults,
    apiFields: STAFF_PROMISE_FORM_FIELDS,
    // `kind` (below) is this render's, the one whose submit sent the request.
    apiError: (error) => staffPromiseFormError(error, kind),
  });
  const kind = offeredPromiseKind(
    useWatch({ control: form.control, name: "kind" }),
    view.offer,
  );

  const submit = form.handleSubmit((body) => {
    if (saving) return;
    onSave(body, showApiError);
  });

  const noteError = form.formState.errors.note?.message;

  return (
    <>
      <PromiseSheetHeader
        title={view.title}
        subtitle={view.subtitle}
        replacesLine={view.replacesLine}
        className={classes.header}
        closeDisabled={saving}
      />

      <form
        id={formId}
        noValidate
        onSubmit={submit}
        data-slot="form-sheet-body"
        className={classes.body}
      >
        {alert !== undefined && <FormAlert message={alert} />}
        {view.intro !== null && (
          <p className="text-sm text-muted-foreground md:text-base">
            {view.intro}
          </p>
        )}

        <Controller
          control={form.control}
          name="kind"
          render={({ field: choice, fieldState }) => (
            <ChoiceRadioGroup
              groupRef={choice.ref}
              legend={PROMISE_KIND_LEGEND}
              legendHidden
              options={view.kindOptions}
              value={kind}
              onChange={(value) => {
                const next = toPromiseKind(value);
                if (next === null) return;
                choice.onChange(next);
                form.clearErrors(["kind", "date"]);
              }}
              error={fieldState.error?.message ?? null}
            />
          )}
        />
        {kind === PromiseKind.specific_date && (
          <Controller
            control={form.control}
            name="date"
            render={({ field: date, fieldState }) => (
              <PromiseDateField
                label={view.dateLabel}
                inputRef={date.ref}
                value={date.value}
                min={view.dateMin}
                max={view.dateMax}
                onChange={(next) => {
                  date.onChange(next);
                  form.clearErrors("date");
                }}
                error={fieldState.error?.message ?? null}
              />
            )}
          />
        )}
        <Controller
          control={form.control}
          name="amountText"
          render={({ field: amount, fieldState }) => (
            <PromiseAmountField
              label={view.amountLabel}
              inputRef={amount.ref}
              text={amount.value}
              hint={view.amountHint}
              onTextChange={(text) => {
                amount.onChange(text);
                form.clearErrors("amountText");
              }}
              error={fieldState.error?.message ?? null}
            />
          )}
        />

        {view.showNote && (
          <div className="flex flex-col gap-2">
            <Label htmlFor={`${id}-note`} className={FIELD_LABEL_CLASS}>
              {PROMISE_NOTE_LABEL}
            </Label>
            <Textarea
              id={`${id}-note`}
              autoComplete="off"
              maxLength={PROMISE_NOTE_MAX_LENGTH}
              placeholder="Contoh: gajian tanggal 10"
              {...field("note")}
              disabled={saving}
              aria-invalid={noteError !== undefined}
              aria-describedby={
                noteError !== undefined ? `${id}-note-error` : undefined
              }
              className={TEXTAREA_CLASS}
            />
            {noteError !== undefined && (
              <FieldError id={`${id}-note-error`} message={noteError} />
            )}
          </div>
        )}
      </form>
      <div data-slot="form-sheet-footer" className={classes.footer}>
        <SheetClose asChild>
          <Button
            type="button"
            variant="outline"
            disabled={saving}
            className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-base font-semibold md:text-[17px]"
          >
            Batal
          </Button>
        </SheetClose>
        <Button
          type="submit"
          form={formId}
          disabled={saving}
          className="h-13 flex-2 rounded-xl px-3 text-base font-semibold whitespace-normal md:text-[17px]"
        >
          {saving ? PROMISE_SAVING_TEXT : view.primaryLabel}
        </Button>
      </div>
    </>
  );
}

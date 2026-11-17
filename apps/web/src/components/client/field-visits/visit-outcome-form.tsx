"use client";

import { useId } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { Label } from "@/components/ui/label";
import { RadioGroup } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { VisitOutcomeKind } from "@/lib/api/generated/model";

import type { RecordVisit } from "./type";
import {
  NOTE_LABEL,
  NOTE_MAX_LENGTH,
  NOTE_OPTIONAL,
  OUTCOME_LEGEND,
  OUTCOME_OPTIONS,
} from "./utils";

import { VisitCashChoice } from "./visit-cash-choice";
import { VisitOutcomeChoice } from "./visit-outcome-choice";
import { VisitPhoneField } from "./visit-phone-field";
import { VisitPromiseChoice } from "./visit-promise-choice";

type VisitOutcomeFormProps = {
  formId: string;
  visit: RecordVisit;
  // "Sisa Rp2.900.000 · telat Rp2.200.000", from the row.
  owed: string;
  canReceiveCash: boolean;
  canPromise: boolean;
  onReceiveCash: () => void;
  onRecordPromise: () => void;
};

// The body of "Catat hasil kunjungan" (board HP-Kunjungan, columns 2 to 5): what the tenant owes,
// the four answers with what each needs inside its card, and "Catatan (boleh kosong)" for every
// answer. Locked after a lost answer: nothing changes until Simpan hasil is answered.
export function VisitOutcomeForm({
  formId,
  visit,
  owed,
  canReceiveCash,
  canPromise,
  onReceiveCash,
  onRecordPromise,
}: VisitOutcomeFormProps) {
  const id = useId();
  const { form, saving, locked } = visit;
  const disabled = saving || locked;
  const { errors } = form.formState;
  const noteError = errors.note?.message;
  const outcomeError = errors.outcome?.message;

  return (
    <form
      id={formId}
      noValidate
      onSubmit={visit.submit}
      data-slot="form-sheet-body"
      className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-5 py-4 md:gap-[18px] md:px-6 md:py-5"
    >
      {visit.alert !== undefined && <FormAlert message={visit.alert} />}
      <p className="text-base font-semibold tabular-nums md:text-[17px]">
        {owed}
      </p>
      <div className="flex flex-col gap-2.5">
        <span
          id={`${id}-legend`}
          className="text-base leading-normal font-semibold md:text-[17px]"
        >
          {OUTCOME_LEGEND}
        </span>
        <Controller
          control={form.control}
          name="outcome"
          render={({ field }) => (
            <RadioGroup
              ref={field.ref}
              tabIndex={-1}
              aria-labelledby={`${id}-legend`}
              aria-describedby={
                outcomeError === undefined ? undefined : `${id}-outcome-error`
              }
              aria-invalid={outcomeError !== undefined}
              value={field.value}
              onValueChange={(value) => {
                field.onChange(value);
                form.clearErrors();
              }}
              disabled={disabled}
              className="gap-2.5 outline-none"
            >
              {OUTCOME_OPTIONS.map((option) => (
                <VisitOutcomeChoice
                  key={option.value}
                  id={`${id}-${option.value}`}
                  option={option}
                  checked={field.value === option.value}
                  disabled={disabled}
                >
                  {option.value === VisitOutcomeKind.paid && (
                    <VisitCashChoice
                      choices={visit.cashChoices}
                      paymentId={visit.paymentId}
                      onChoose={visit.choosePayment}
                      disabled={disabled}
                      canReceive={canReceiveCash}
                      onReceiveCash={onReceiveCash}
                    />
                  )}
                  {option.value === VisitOutcomeKind.promise && (
                    <VisitPromiseChoice
                      promise={visit.promise}
                      canPromise={canPromise}
                      onRecordPromise={onRecordPromise}
                      disabled={disabled}
                    />
                  )}
                  {option.value === VisitOutcomeKind.number_changed && (
                    <VisitPhoneField
                      registration={form.register("newPhone", {
                        onChange: () => form.clearErrors("newPhone"),
                      })}
                      error={errors.newPhone?.message}
                      disabled={disabled}
                    />
                  )}
                </VisitOutcomeChoice>
              ))}
            </RadioGroup>
          )}
        />
        {outcomeError !== undefined && (
          <FieldError id={`${id}-outcome-error`} message={outcomeError} />
        )}
      </div>
      <div className="flex flex-col gap-2">
        <Label htmlFor={`${id}-note`} className={FIELD_LABEL_CLASS}>
          {NOTE_LABEL}{" "}
          <span className="font-normal text-muted-foreground">
            {NOTE_OPTIONAL}
          </span>
        </Label>
        <Textarea
          id={`${id}-note`}
          autoComplete="off"
          maxLength={NOTE_MAX_LENGTH}
          {...form.register("note", {
            onChange: () => form.clearErrors("note"),
          })}
          disabled={disabled}
          aria-invalid={noteError !== undefined}
          aria-describedby={
            noteError === undefined ? undefined : `${id}-note-error`
          }
          className={TEXTAREA_CLASS}
        />
        {noteError !== undefined && (
          <FieldError id={`${id}-note-error`} message={noteError} />
        )}
      </div>
    </form>
  );
}

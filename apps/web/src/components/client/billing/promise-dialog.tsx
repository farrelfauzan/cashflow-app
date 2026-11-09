"use client";

import { useQueryClient } from "@tanstack/react-query";
import { XIcon } from "lucide-react";
import { useId } from "react";
import { Controller, useWatch } from "react-hook-form";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { ChoiceRadioGroup } from "@/components/client/payment-page/choice-radio-group";
import { PromiseAmountField } from "@/components/client/payment-page/promise-amount-field";
import { PromiseDateField } from "@/components/client/payment-page/promise-date-field";
import {
  offeredPromiseKind,
  PROMISE_KIND_LEGEND,
  PROMISE_SAVING_TEXT,
  toPromiseKind,
} from "@/components/client/payment-page/utils";
import { notify } from "@/components/client/toast/notify";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  SheetClose,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import {
  getGetTenantAccountQueryKey,
  getListTenantPromisesQueryKey,
  useMakeTenantPromise,
} from "@/lib/api/generated/money/money";
import { PromiseKind } from "@/lib/api/generated/model";
import type { PromiseDialogView } from "./type";
import {
  BILLING_DIALOG_BODY_CLASS,
  BILLING_DIALOG_FOOTER_CLASS,
  BILLING_DIALOG_HEADER_CLASS,
  isInvoicesQuery,
  isPromiseStaleError,
  PROMISE_NOTE_LABEL,
  PROMISE_NOTE_MAX_LENGTH,
  RECORD_PROMISE_TEXT,
  STAFF_PROMISE_FORM_FIELDS,
  staffPromiseFormError,
  staffPromiseSchema,
  promiseSavedToast,
} from "./utils";

type PromiseDialogProps = {
  orgId: string;
  tenantId: string;
  // Made from the account when the dialog opened.
  view: PromiseDialogView;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // After the promise is saved and the queries that show it are reloaded.
  onSaved: () => void;
};

// "Catat janji" (no board: a design gap, built like "Koreksi nominal" with the fields of the
// tenant's Janji screen): when the tenant pays what is left and, optionally, how much, with the
// member's note. Over an active promise it opens filled from it and saving adds a version. It
// never sends payingNow. It stays open while the request runs. A centred dialog from md up; on
// the phone a form sheet (board HP-Lembar) with Batal and the save button pinned at the bottom,
// which asks before a changed promise is closed unsaved.
export function PromiseDialog({
  orgId,
  tenantId,
  view,
  open,
  onOpenChange,
  onSaved,
}: PromiseDialogProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
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

  const reloadAccount = () =>
    queryClient.invalidateQueries({
      queryKey: getGetTenantAccountQueryKey(orgId, tenantId),
    });
  // The account's tile, "Riwayat janji" and Penagihan's rows show the promise. Nothing cached is
  // patched; the API's answer is the one shown.
  const make = useMakeTenantPromise({
    mutation: {
      // The toast is raised here, not in mutate()'s callback, so it shows even if the dialog is
      // gone by the time the API answers. Over an active promise, saving replaced it.
      onSuccess: () => {
        notify(promiseSavedToast(view.replacesLine !== null, view.subtitle));
        return Promise.all([
          reloadAccount(),
          queryClient.invalidateQueries({
            queryKey: getListTenantPromisesQueryKey(orgId, tenantId),
          }),
          queryClient.invalidateQueries({
            predicate: (query) => isInvoicesQuery(query.queryKey, orgId),
          }),
        ]);
      },
    },
  });
  const saving = make.isPending;

  const submit = form.handleSubmit((body) => {
    if (saving) return;
    make.mutate(
      { orgId, tenantId, data: body },
      {
        onSuccess: onSaved,
        onError: (error) => {
          // What the page shows behind the dialog is out of date; the dialog keeps the message.
          if (isPromiseStaleError(error)) void reloadAccount();
          showApiError(error);
        },
      },
    );
  });

  const noteError = form.formState.errors.note?.message;

  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      dirty={form.formState.isDirty}
      busy={saving}
    >
      <FormSheetContent wide="dialog" showCloseButton={false}>
        <SheetHeader className={BILLING_DIALOG_HEADER_CLASS}>
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <SheetTitle className="text-xl leading-tight font-bold md:text-[22px]">
              {RECORD_PROMISE_TEXT}
            </SheetTitle>
            <SheetDescription className="text-sm text-muted-foreground md:text-base">
              {view.subtitle}
            </SheetDescription>
            {view.replacesLine !== null && (
              <p className="text-sm font-semibold text-warning md:text-base">
                {view.replacesLine}
              </p>
            )}
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Tutup"
              disabled={saving}
              className="-mt-1.5 -mr-2 rounded-full"
            >
              <XIcon aria-hidden="true" className="size-5" />
            </Button>
          </SheetClose>
        </SheetHeader>

        <form
          id={formId}
          noValidate
          onSubmit={submit}
          className={BILLING_DIALOG_BODY_CLASS}
        >
          {alert !== undefined && <FormAlert message={alert} />}

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
        </form>
        <div className={BILLING_DIALOG_FOOTER_CLASS}>
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
      </FormSheetContent>
    </FormSheet>
  );
}

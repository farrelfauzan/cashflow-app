"use client";

import { XIcon } from "lucide-react";
import { useId, useRef, type FormEvent } from "react";
import { Controller, useWatch } from "react-hook-form";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
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
import { useConfirmCashDeposit } from "@/lib/api/generated/money/money";
import type { CashDeposit } from "@/lib/api/generated/model";

import type { CountFormOutput } from "./type";
import {
  ALREADY_COUNTED_TOAST,
  CANCEL_LABEL,
  confirmErrorKind,
  confirmRequestBody,
  COUNT_API_FIELDS,
  COUNT_FORM_DEFAULTS,
  countFormSchema,
  countHint,
  countSavedToast,
  countSheetView,
  NOTE_LABEL,
  NOTE_MAX_LENGTH,
  RECEIVED_HINT,
  RECEIVED_LABEL,
  SAVE_LABEL,
  SAVING_LABEL,
  SHEET_TITLE,
} from "./utils";

import { CountHintBox } from "./count-hint-box";
import { DepositFigures } from "./deposit-figures";
import { DepositReceiptList } from "./deposit-receipt-list";
import { useRefreshDeposits } from "./use-refresh-deposits";

type CountDepositSheetProps = {
  orgId: string;
  deposit: CashDeposit;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

// "Hitung setoran" (board W20b drawer): the receipts' total and what the karyawan says they
// handed over, the receipts, and the amount the admin counted, which opens empty so they type
// their own count. A line under it compares the typed amount with the receipts' total. The API
// saves the count once and works out the gap. The caller mounts it afresh for every opening. It
// stays open while the request runs; on the phone it is the bottom sheet, which asks before a
// typed count is closed unsaved.
export function CountDepositSheet({
  orgId,
  deposit,
  open,
  onOpenChange,
}: CountDepositSheetProps) {
  const id = useId();
  const formId = `${id}-form`;
  const view = countSheetView(deposit);
  const refresh = useRefreshDeposits(orgId);
  const { form, field, showApiError, alert } = useApiForm({
    schema: countFormSchema,
    defaultValues: COUNT_FORM_DEFAULTS,
    apiFields: COUNT_API_FIELDS,
  });
  const received = useWatch({ control: form.control, name: "receivedAmount" });
  const hint = countHint(received, deposit.expectedAmount);

  // Both lists and Penagihan's count are reloaded; the toast is raised here, so it shows
  // whatever happens to the drawer.
  const save = useConfirmCashDeposit({
    mutation: {
      onSuccess: (counted) => {
        notify(countSavedToast(counted));
        return refresh();
      },
    },
  });
  const saving = save.isPending || save.isSuccess;

  // Set the moment a request goes out, before React renders `saving`: two quick presses both pass
  // the (async) checks, and only the first may send. Cleared when the API refuses.
  const sent = useRef(false);

  function refused(error: unknown) {
    sent.current = false;
    if (confirmErrorKind(error) === "already_confirmed") {
      // Someone counted it first: the lists show their count.
      notify(ALREADY_COUNTED_TOAST);
      void refresh();
      onOpenChange(false);
      return;
    }
    showApiError(error);
  }

  function send(fields: CountFormOutput) {
    if (saving || sent.current) return;
    sent.current = true;
    save.mutate(
      { orgId, depositId: deposit.id, data: confirmRequestBody(fields) },
      { onSuccess: () => onOpenChange(false), onError: refused },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  const noteError = form.formState.errors.note?.message;

  return (
    <FormSheet
      open={open}
      onOpenChange={onOpenChange}
      dirty={form.formState.isDirty}
      busy={save.isPending}
    >
      <FormSheetContent
        showCloseButton={false}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          form.setFocus("receivedAmount");
        }}
      >
        <SheetHeader className="flex-row items-start gap-3 border-b px-6 pt-6 pb-4">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <SheetTitle className="text-2xl font-bold">
              {SHEET_TITLE}
            </SheetTitle>
            <SheetDescription className="text-base text-muted-foreground">
              {view.subtitle}
            </SheetDescription>
          </div>
          <SheetClose asChild>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              aria-label="Tutup"
              disabled={save.isPending}
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
          data-slot="form-sheet-body"
          className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
        >
          {alert !== undefined && <FormAlert message={alert} />}
          <DepositFigures view={view} />
          <DepositReceiptList orgId={orgId} depositId={deposit.id} />
          <Controller
            control={form.control}
            name="receivedAmount"
            render={({ field: amount, fieldState }) => (
              <RupiahField
                inputRef={amount.ref}
                label={RECEIVED_LABEL}
                name="receivedAmount"
                placeholder="0"
                hint={RECEIVED_HINT}
                value={amount.value}
                onChange={(text) => {
                  amount.onChange(text);
                  form.clearErrors("receivedAmount");
                }}
                error={fieldState.error?.message}
                disabled={saving}
              />
            )}
          />
          <CountHintBox hint={hint} />
          <div className="flex flex-col gap-2">
            <Label htmlFor={`${id}-note`} className={FIELD_LABEL_CLASS}>
              {NOTE_LABEL}
            </Label>
            <Textarea
              id={`${id}-note`}
              autoComplete="off"
              maxLength={NOTE_MAX_LENGTH}
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
        <div
          data-slot="form-sheet-footer"
          className="flex gap-2.5 border-t px-6 py-4"
        >
          <SheetClose asChild>
            <Button
              type="button"
              variant="outline"
              disabled={save.isPending}
              className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
            >
              {CANCEL_LABEL}
            </Button>
          </SheetClose>
          <Button
            type="submit"
            form={formId}
            disabled={saving}
            className="h-13 flex-2 rounded-xl px-3 text-base font-semibold whitespace-normal md:text-[17px]"
          >
            {saving ? SAVING_LABEL : SAVE_LABEL}
          </Button>
        </div>
      </FormSheetContent>
    </FormSheet>
  );
}

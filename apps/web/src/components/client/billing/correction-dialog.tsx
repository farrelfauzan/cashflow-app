"use client";

import { useQueryClient } from "@tanstack/react-query";
import { XIcon } from "lucide-react";
import { useId, useState } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahInput } from "@/components/client/form/rupiah-input";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_LABEL_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { LeaseInfoNote } from "@/components/client/lease/lease-info-note";
import { notify } from "@/components/client/toast/notify";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  getGetDashboardQueryKey,
  getGetTenantAccountQueryKey,
  getListTenantOpenInvoicesQueryKey,
  useCreateCorrection,
} from "@/lib/api/generated/money/money";
import type { CorrectionDialogView } from "./type";
import {
  CORRECTION_FORM_API_FIELDS,
  CORRECTION_NOTE,
  correctionFormError,
  correctionFormSchema,
  correctionInput,
  isInvoicesQuery,
  REASON_MAX_LENGTH,
  correctionSavedToast,
} from "./utils";

type CorrectionDialogProps = {
  orgId: string;
  tenantId: string;
  view: CorrectionDialogView;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // After the correction is saved and the money it changes is reloaded.
  onSaved: () => void;
};

// "Koreksi nominal" for one invoice or payment (board "iPad · Koreksi nominal"): the amount now,
// the new amount and a required reason. It sends the new amount; the API takes the difference
// and adds a correction row, so nothing is edited. The caller mounts it afresh for every opening,
// so one requestId covers the retries of that opening. It stays open while the request runs.
export function CorrectionDialog({
  orgId,
  tenantId,
  view,
  open,
  onOpenChange,
  onSaved,
}: CorrectionDialogProps) {
  const id = useId();
  const [requestId] = useState(() => crypto.randomUUID());
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: correctionFormSchema,
    defaultValues: { newAmount: "", reason: "" },
    apiFields: CORRECTION_FORM_API_FIELDS,
    apiError: correctionFormError,
  });

  // The tenant's account and open invoices ("Salin link bayar"), Beranda and Penagihan all show
  // what a correction changes. Cached money is never patched; the API's answer is the one shown.
  const create = useCreateCorrection({
    mutation: {
      // The toast is raised here, not in mutate()'s callback, so it shows even if the dialog is
      // gone by the time the API answers.
      onSuccess: () => {
        notify(correctionSavedToast(view.title));
        return Promise.all([
          queryClient.invalidateQueries({
            queryKey: getGetTenantAccountQueryKey(orgId, tenantId),
          }),
          queryClient.invalidateQueries({
            queryKey: getListTenantOpenInvoicesQueryKey(orgId, tenantId),
          }),
          queryClient.invalidateQueries({
            queryKey: getGetDashboardQueryKey(orgId),
          }),
          queryClient.invalidateQueries({
            predicate: (query) => isInvoicesQuery(query.queryKey, orgId),
          }),
        ]);
      },
    },
  });
  const saving = create.isPending;

  const submit = form.handleSubmit((values) => {
    if (saving) return;
    create.mutate(
      { orgId, data: correctionInput(requestId, view.target, values) },
      { onSuccess: onSaved, onError: showApiError },
    );
  });

  const reasonError = form.formState.errors.reason?.message;

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        if (!saving) onOpenChange(next);
      }}
    >
      <DialogContent
        showCloseButton={false}
        onOpenAutoFocus={(event) => {
          event.preventDefault();
          form.setFocus("newAmount");
        }}
        className="flex max-h-[calc(100dvh-2rem)] max-w-[calc(100%-2rem)] flex-col gap-0 overflow-y-auto rounded-2xl bg-card p-0 sm:max-w-[560px]"
      >
        <div className="flex items-start gap-3 px-5 pt-5 md:px-6 md:pt-6">
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <DialogTitle className="text-xl leading-tight font-bold md:text-[22px]">
              {view.title}
            </DialogTitle>
            <DialogDescription className="text-sm text-muted-foreground md:text-base">
              {view.subtitle}
            </DialogDescription>
          </div>
          <DialogClose asChild>
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
          </DialogClose>
        </div>

        <form
          noValidate
          onSubmit={submit}
          className="flex flex-col gap-[18px] px-5 pt-5 pb-5 md:px-6 md:pb-6"
        >
          {alert !== undefined && <FormAlert message={alert} />}

          <div className="flex items-center justify-between gap-3 rounded-xl bg-muted px-4 py-3">
            <span className="text-[15px] md:text-base">Nominal sekarang</span>
            <span className="text-base font-bold md:text-[17px]">
              {view.current}
            </span>
          </div>

          <Controller
            control={form.control}
            name="newAmount"
            render={({ field: amount, fieldState }) => (
              <div className="flex flex-col gap-2">
                <Label htmlFor={`${id}-amount`} className={FIELD_LABEL_CLASS}>
                  Nominal baru
                </Label>
                <RupiahInput
                  inputRef={amount.ref}
                  id={`${id}-amount`}
                  name="newAmount"
                  placeholder="0"
                  value={amount.value}
                  onChange={(text) => {
                    amount.onChange(text);
                    form.clearErrors("newAmount");
                  }}
                  invalid={fieldState.error !== undefined}
                  describedBy={
                    fieldState.error !== undefined
                      ? `${id}-amount-error`
                      : undefined
                  }
                  disabled={saving}
                  className="h-14"
                  textClassName="text-xl font-semibold md:text-xl"
                />
                {fieldState.error?.message !== undefined && (
                  <FieldError
                    id={`${id}-amount-error`}
                    message={fieldState.error.message}
                  />
                )}
              </div>
            )}
          />

          <div className="flex flex-col gap-2">
            <Label htmlFor={`${id}-reason`} className={FIELD_LABEL_CLASS}>
              Alasan (wajib diisi)
            </Label>
            <Textarea
              id={`${id}-reason`}
              autoComplete="off"
              maxLength={REASON_MAX_LENGTH}
              placeholder="Contoh: potongan karena kran rusak"
              {...field("reason")}
              disabled={saving}
              aria-invalid={reasonError !== undefined}
              aria-describedby={
                reasonError !== undefined ? `${id}-reason-error` : undefined
              }
              className={TEXTAREA_CLASS}
            />
            {reasonError !== undefined && (
              <FieldError id={`${id}-reason-error`} message={reasonError} />
            )}
          </div>

          <LeaseInfoNote text={CORRECTION_NOTE} />

          <div className="flex gap-2.5 pt-1">
            <DialogClose asChild>
              <Button
                type="button"
                variant="outline"
                disabled={saving}
                className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-base font-semibold md:text-[17px]"
              >
                Batal
              </Button>
            </DialogClose>
            <Button
              type="submit"
              variant="caution"
              disabled={saving}
              className="h-13 flex-2 rounded-xl px-3 text-base font-semibold whitespace-normal md:text-[17px]"
            >
              {saving ? "Menyimpan…" : "Simpan koreksi"}
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}

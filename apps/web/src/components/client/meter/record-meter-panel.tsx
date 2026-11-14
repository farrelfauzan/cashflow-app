"use client";

import { useQueryClient } from "@tanstack/react-query";
import {
  useEffect,
  useEffectEvent,
  useId,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { Controller, useWatch } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
} from "@/components/client/form/utils";
import { LeaseInfoNote } from "@/components/client/lease/lease-info-note";
import { LeasePanelFooter } from "@/components/client/lease/lease-panel-footer";
import { notify } from "@/components/client/toast/notify";
import { toFormError } from "@/lib/api/form-error";
import type { Lease, MeteredCharge } from "@/lib/api/generated/model";
import {
  getGetTenantAccountQueryKey,
  getListLeaseMeteredChargesQueryKey,
  useListLeaseMeteredCharges,
  useRecordMeteredCharge,
} from "@/lib/api/generated/money/money";
import type {
  MeterFormOutput,
  MeterSelection,
  RecordedMeterRowView,
} from "./type";
import {
  AMOUNT_HINT,
  AMOUNT_LABEL,
  existingAmountText,
  findCharge,
  isBilledError,
  isMissingMetersQueryKey,
  lockedSelectionText,
  lockedView,
  METER_API_FIELDS,
  meterFieldValues,
  meterFormDefaults,
  meterFormSchema,
  METER_NUMBERS_HINT,
  METER_NUMBERS_LABEL,
  meterRequest,
  MONTH_HINT,
  nextInvoiceText,
  RECORDED_PAGE_SIZE,
  recordedRows,
  SAVE_CHANGE_LABEL,
  SAVE_NEW_LABEL,
  savedText,
  selectionKey,
  usageMonthOptions,
  meterRecordedToast,
} from "./utils";

import { MeterLockedView } from "./meter-locked-view";
import { MeterNumberInput } from "./meter-number-input";
import { MeterRecordedList } from "./meter-recorded-list";
import { MeterStatusBox } from "./meter-status-box";
import { MeterTypeField } from "./meter-type-field";

type RecordMeterPanelProps = {
  orgId: string;
  lease: Lease;
  // "Today" in Jakarta: which usage months are offered.
  today: string;
  // "Koreksi tagihan …" on a locked amount; null hides it (no correction:create).
  onCorrectInvoice: ((invoiceId: string) => void) | null;
};

// "Catat meteran" (boards "W14 iPad · Catat meteran" and "Meteran sudah ditagihkan"), in the
// tenant page's panel: a month, a metered type, the optional meter numbers and the rupiah amount
// the admin types. A month and type already recorded but not billed is prefilled and replaced on
// save; a billed one shows read only, with the invoice to correct instead. After a save the panel
// stays open for the next reading. Below the form, "Sudah dicatat" lists the lease's amounts.
export function RecordMeterPanel({
  orgId,
  lease,
  today,
  onCorrectInvoice,
}: RecordMeterPanelProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const [months] = useState(() => usageMonthOptions(lease.startDate, today));
  const { form, showApiError, alert } = useApiForm({
    schema: meterFormSchema,
    defaultValues: meterFormDefaults(months, lease),
    apiFields: METER_API_FIELDS,
  });
  const [usageMonth, chargeTypeId] = useWatch({
    control: form.control,
    name: ["usageMonth", "chargeTypeId"],
  });
  const selection: MeterSelection = { usageMonth, chargeTypeId };
  const key = selectionKey(selection);

  // The lease's amounts (first page) and the invoice the next amount goes on.
  const recorded = useListLeaseMeteredCharges(orgId, lease.id, {
    pageSize: RECORDED_PAGE_SIZE,
  });
  // The chosen month and type, whatever page it would be on.
  const chosen = useListLeaseMeteredCharges(orgId, lease.id, {
    chargeTypeId,
    usageMonth,
  });
  const chosenLoaded = chosen.data !== undefined;
  const existing =
    findCharge(chosen.data?.items, selection) ??
    (chosenLoaded ? undefined : findCharge(recorded.data?.items, selection));
  const known = chosenLoaded || existing !== undefined;
  const billed = existing?.billed !== undefined ? existing : undefined;

  // The last amount saved in this opening ("tersimpan"), and the selection whose locked view the
  // member went back with "Kembali".
  const [saved, setSaved] = useState<MeteredCharge | null>(null);
  const [dismissed, setDismissed] = useState<string | null>(null);

  // When the month or type changes, the amount and meter fields empty at once; once the API says
  // what is recorded for it, an amount not billed yet is prefilled for a change, but only into
  // fields still empty, so a late answer never overwrites what the member typed meanwhile.
  // Applied once per selection, so a refetch after a save does not refill the cleared fields.
  const applied = useRef<string | null>(null);
  function fillFields(charge: MeteredCharge | undefined) {
    const next = meterFieldValues(
      charge?.billed === undefined ? charge : undefined,
    );
    form.setValue("amount", next.amount);
    form.setValue("meterStart", next.meterStart);
    form.setValue("meterEnd", next.meterEnd);
    form.clearErrors(["amount", "meterStart", "meterEnd"]);
  }
  const prefill = useEffectEvent((charge: MeteredCharge | undefined) => {
    if (charge === undefined || charge.billed !== undefined) return;
    const typed = form.getValues(["amount", "meterStart", "meterEnd"]);
    if (typed.some((value) => value !== "")) return;
    fillFields(charge);
  });
  useEffect(() => {
    if (!known || applied.current === key) return;
    applied.current = key;
    prefill(existing);
  }, [key, known, existing]);

  // Every place that shows a metered amount: this lease's list, the tenant's account (an amount
  // never changes money already billed, but the page stays current) and Penagihan's flag. Cached
  // money is never patched.
  async function refresh() {
    await Promise.all([
      queryClient.invalidateQueries({
        queryKey: getListLeaseMeteredChargesQueryKey(orgId, lease.id),
      }),
      queryClient.invalidateQueries({
        queryKey: getGetTenantAccountQueryKey(orgId, lease.tenantId),
      }),
      queryClient.invalidateQueries({
        predicate: (query) => isMissingMetersQueryKey(query.queryKey, orgId),
      }),
    ]);
  }

  const save = useRecordMeteredCharge({
    mutation: {
      onSuccess: (charge) => {
        notify(meterRecordedToast(charge));
        return refresh();
      },
    },
  });
  const saving = save.isPending;

  function send(fields: MeterFormOutput) {
    if (saving) return;
    // A billed month and type cannot change: show why instead of sending.
    if (billed !== undefined) {
      setDismissed(null);
      return;
    }
    const request = meterRequest(fields);
    save.mutate(
      { orgId, leaseId: lease.id, ...request },
      {
        onSuccess: (charge) => {
          setSaved(charge);
          applied.current = selectionKey(request);
          fillFields(undefined);
          // Saved: what is left (the month and type) is the form's new starting point, so
          // closing the sheet now does not ask about unsaved changes.
          form.reset(form.getValues());
        },
        onError: async (error) => {
          if (isBilledError(error)) {
            // An invoice billed it meanwhile: reload, then show it locked.
            await refresh();
            const fresh = await chosen.refetch();
            if (findCharge(fresh.data?.items, request)?.billed !== undefined) {
              setDismissed(null);
              return;
            }
          }
          showApiError(error);
        },
      },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  function select(next: MeterSelection) {
    form.setValue("usageMonth", next.usageMonth);
    form.setValue("chargeTypeId", next.chargeTypeId);
    form.clearErrors(["usageMonth", "chargeTypeId"]);
    fillFields(undefined);
  }

  function openRow(row: RecordedMeterRowView) {
    const charge = findCharge(recorded.data?.items, row.selection);
    select(row.selection);
    setDismissed(null);
    // "Ubah" on the month and type already chosen (just saved) prefills it again.
    if (selectionKey(row.selection) === key && charge !== undefined) {
      applied.current = key;
      fillFields(charge);
    }
  }

  const locked =
    billed !== undefined && dismissed !== key ? lockedView(billed) : null;
  if (locked !== null) {
    const invoiceId = locked.invoiceId;
    return (
      <MeterLockedView
        view={locked}
        onCorrect={
          onCorrectInvoice === null ? null : () => onCorrectInvoice(invoiceId)
        }
        onClose={() => setDismissed(key)}
      />
    );
  }

  const savedHere =
    saved !== null && selectionKey(saved) === key && existing?.id === saved.id;
  let status: { tone: "warning" | "success" | "locked"; text: string } | null =
    null;
  if (savedHere) {
    status = {
      tone: "success",
      text: savedText(saved, recorded.data?.nextInvoice),
    };
  } else if (billed !== undefined) {
    status = { tone: "locked", text: lockedSelectionText(billed) };
  } else if (existing !== undefined) {
    status = { tone: "warning", text: existingAmountText(existing) };
  } else if (saved !== null) {
    status = {
      tone: "success",
      text: savedText(saved, recorded.data?.nextInvoice),
    };
  }

  const meterHintId = `${id}-meter-hint`;
  const meterStartError = form.formState.errors.meterStart?.message;
  const meterEndError = form.formState.errors.meterEnd?.message;

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        data-slot="form-sheet-body"
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}
        {status !== null && (
          <MeterStatusBox tone={status.tone} text={status.text} />
        )}
        <Controller
          control={form.control}
          name="usageMonth"
          render={({ field, fieldState }) => (
            <SelectField
              triggerRef={field.ref}
              label="Bulan pemakaian"
              value={field.value}
              placeholder="Pilih bulan"
              options={months}
              onValueChange={(value) =>
                select({ usageMonth: value, chargeTypeId })
              }
              hint={MONTH_HINT}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <Controller
          control={form.control}
          name="chargeTypeId"
          render={({ field, fieldState }) => (
            <MeterTypeField
              inputRef={field.ref}
              types={lease.meteredChargeTypes}
              value={field.value}
              onChange={(value) => select({ usageMonth, chargeTypeId: value })}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <div
          role="group"
          aria-labelledby={`${id}-meter-label`}
          className="flex flex-col gap-2"
        >
          <span id={`${id}-meter-label`} className={FIELD_LABEL_CLASS}>
            {METER_NUMBERS_LABEL}
          </span>
          <div className="flex gap-2.5">
            <Controller
              control={form.control}
              name="meterStart"
              render={({ field }) => (
                <MeterNumberInput
                  inputRef={field.ref}
                  label="Awal"
                  name="meterStart"
                  value={field.value}
                  onChange={(digits) => {
                    field.onChange(digits);
                    form.clearErrors("meterStart");
                  }}
                  error={meterStartError}
                  hintId={meterHintId}
                  disabled={saving}
                />
              )}
            />
            <Controller
              control={form.control}
              name="meterEnd"
              render={({ field }) => (
                <MeterNumberInput
                  inputRef={field.ref}
                  label="Akhir"
                  name="meterEnd"
                  value={field.value}
                  onChange={(digits) => {
                    field.onChange(digits);
                    form.clearErrors("meterEnd");
                  }}
                  error={meterEndError}
                  hintId={meterHintId}
                  disabled={saving}
                />
              )}
            />
          </div>
          <p id={meterHintId} className={FIELD_HINT_CLASS}>
            {METER_NUMBERS_HINT}
          </p>
        </div>
        <Controller
          control={form.control}
          name="amount"
          render={({ field, fieldState }) => (
            <RupiahField
              inputRef={field.ref}
              label={AMOUNT_LABEL}
              name="amount"
              placeholder="0"
              hint={AMOUNT_HINT}
              value={field.value}
              onChange={(text) => {
                field.onChange(text);
                form.clearErrors("amount");
              }}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        {recorded.data !== undefined && (
          <LeaseInfoNote text={nextInvoiceText(recorded.data.nextInvoice)} />
        )}
        <MeterRecordedList
          rows={
            recorded.data === undefined
              ? null
              : recordedRows(recorded.data.items)
          }
          error={
            recorded.isError ? toFormError(recorded.error, []).message : null
          }
          onOpen={openRow}
        />
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel={saved === null ? "Batal" : "Selesai"}
        // Right after a save the fields are empty for the next reading (board "saved" state), so
        // the label is "Simpan meteran" again, as the warning is hidden for that reading.
        submitLabel={
          existing === undefined || savedHere
            ? SAVE_NEW_LABEL
            : SAVE_CHANGE_LABEL
        }
        saving={saving}
        wide={false}
      />
    </>
  );
}

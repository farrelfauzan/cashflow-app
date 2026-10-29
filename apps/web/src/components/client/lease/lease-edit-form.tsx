"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { DateField } from "@/components/client/form/date-field";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { toFormError } from "@/lib/api/form-error";
import { useListCollectorCandidates } from "@/lib/api/generated/members/members";
import type { Lease } from "@/lib/api/generated/model";
import { useUpdateLease } from "@/lib/api/generated/tenants/tenants";
import type { BillingChoice, LeaseFormErrors, LeaseFormField } from "./type";
import {
  BOOKING_NOTE,
  COLLECTOR_HINT,
  DEPOSIT_HINT,
  DISABLED_COLLECTOR_HINT,
  EDIT_LEASE_NOTE,
  LEASE_TERMS_API_FIELDS,
  NO_COLLECTOR_LABEL,
  billedFromOptions,
  checkLeaseTerms,
  collectorOptions,
  defaultBilledFrom,
  editStartBox,
  firstLeaseError,
  isLeaseFormField,
  keepsDisabledCollector,
  leaseTermsValues,
  leaseUnitText,
  startDateHint,
} from "./utils";

import { LeaseBillingField } from "./lease-billing-field";
import { LeaseInfoNote } from "./lease-info-note";
import { LeasePanelFooter } from "./lease-panel-footer";
import { useRefreshLease } from "./use-refresh-lease";

type LeaseEditFormProps = {
  orgId: string;
  lease: Lease;
  tenantName: string;
  // "Today" in Jakarta, read once when the page opened.
  today: string;
  onSaved: (lease: Lease) => void;
};

// "Ubah sewa" (boards Sewa-Ubah.dc.html and Sewa-Ubah-Phone.dc.html): the start date, "Mulai
// tagih dari" for a lease entered late, the deposit and the collector of an active lease. The
// unit, the tenant and the term never change. On iPad the billing choice has the right half of
// the wide panel to itself, as in "Sewakan unit"; on the phone the fields stack in the board's
// order.
export function LeaseEditForm({
  orgId,
  lease,
  tenantName,
  today,
  onSaved,
}: LeaseEditFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshLease(orgId);
  const startRef = useRef<HTMLInputElement>(null);
  const billedFromRef = useRef<HTMLButtonElement>(null);
  const depositRef = useRef<HTMLInputElement>(null);
  const collectorRef = useRef<HTMLButtonElement>(null);

  const [initial] = useState(() => leaseTermsValues(lease));
  const [startDate, setStartDate] = useState(initial.startDate);
  const [billing, setBilling] = useState<BillingChoice>(initial.billing);
  const [chosenBilledFrom, setChosenBilledFrom] = useState(initial.billedFrom);
  const [deposit, setDeposit] = useState(initial.deposit);
  const [collector, setCollector] = useState(initial.collector);
  const [errors, setErrors] = useState<LeaseFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

  // The candidates load again whenever the form opens, so a member enabled or disabled since the
  // page cached them shows as they are now.
  const candidates = useListCollectorCandidates(orgId, {
    query: { refetchOnMount: "always" },
  });

  const box = editStartBox(startDate, lease.enteredOn, today);
  const options =
    box === "billing" ? billedFromOptions(startDate, lease.enteredOn) : [];
  const billedFrom = options.some((o) => o.value === chosenBilledFrom)
    ? chosenBilledFrom
    : defaultBilledFrom(options, lease.enteredOn);
  const disabledCollector = keepsDisabledCollector(collector, lease.collector);

  const update = useUpdateLease({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = update.isPending;

  function clearError(field: LeaseFormField) {
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  // The field to focus after a refused save. The fields are disabled while saving, so the focus
  // waits until they are enabled again; a new object each time, so the same field can repeat.
  const [focusTarget, setFocusTarget] = useState<{
    field: LeaseFormField | null;
  } | null>(null);
  useEffect(() => {
    if (focusTarget === null || saving) return;
    const ref = {
      propertyId: null,
      unitId: null,
      startDate: startRef,
      billedFrom: billedFromRef,
      depositAmount: depositRef,
      collectorMemberId: collectorRef,
    };
    if (focusTarget.field !== null) ref[focusTarget.field]?.current?.focus();
  }, [focusTarget, saving]);

  function showErrors(found: LeaseFormErrors) {
    setErrors(found);
    setFocusTarget({ field: firstLeaseError(found) });
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, LEASE_TERMS_API_FIELDS);
    if (isLeaseFormField(failed.field)) {
      showErrors({ [failed.field]: failed.message });
    } else {
      setAlert(failed.message);
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkLeaseTerms(
      { startDate, billing, billedFrom, deposit, collector },
      lease,
    );
    if (!checked.ok) {
      showErrors(checked.errors);
      return;
    }
    setErrors({});
    update.mutate(
      { orgId, leaseId: lease.id, data: checked.input },
      { onSuccess: onSaved, onError: handleError },
    );
  }

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={handleSubmit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== null && <FormAlert message={alert} />}

        <div className="flex flex-col gap-[18px] md:grid md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,2fr)] md:grid-rows-[auto_auto_auto_auto_1fr] md:items-start md:gap-x-4 md:[grid-template-areas:'box_box_note'_'deposit_deposit_billed'_'start_start_billed'_'collector_collector_billed'_'._._billed']">
          <div className="flex min-w-0 flex-col gap-1 rounded-xl bg-muted p-3.5 text-base md:text-[17px] md:[grid-area:box]">
            <p>
              <span className="text-muted-foreground">Unit: </span>
              <span className="font-semibold">{leaseUnitText(lease)}</span>
            </p>
            <p>
              <span className="text-muted-foreground">Penyewa: </span>
              <span className="font-semibold">{tenantName}</span>
            </p>
          </div>

          <DateField
            inputRef={startRef}
            label="Mulai sewa"
            name="startDate"
            value={startDate}
            onChange={(next) => {
              setStartDate(next);
              clearError("startDate");
              clearError("billedFrom");
            }}
            hint={startDateHint(startDate)}
            error={errors.startDate}
            disabled={saving}
            className="md:[grid-area:start]"
          />

          <div className="min-w-0 md:[grid-area:note]">
            <LeaseInfoNote text={EDIT_LEASE_NOTE} />
          </div>

          {box === "billing" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseBillingField
                selectRef={billedFromRef}
                startDate={startDate}
                enteredOn={lease.enteredOn}
                today={today}
                billing={billing}
                onBillingChange={(next) => {
                  setBilling(next);
                  clearError("billedFrom");
                }}
                options={options}
                billedFrom={billedFrom}
                onBilledFromChange={(next) => {
                  setChosenBilledFrom(next);
                  clearError("billedFrom");
                }}
                error={errors.billedFrom}
                disabled={saving}
              />
            </div>
          )}
          {box === "booking" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseInfoNote text={BOOKING_NOTE} />
            </div>
          )}
          {box !== "billing" && errors.billedFrom !== undefined && (
            <div className="min-w-0 md:[grid-area:billed]">
              <FieldError
                id={`${id}-billed-error`}
                message={errors.billedFrom}
              />
            </div>
          )}

          <div className="min-w-0 md:[grid-area:deposit]">
            <RupiahField
              inputRef={depositRef}
              label="Uang jaminan"
              name="depositAmount"
              placeholder="0"
              hint={DEPOSIT_HINT}
              value={deposit}
              onChange={(text) => {
                setDeposit(text);
                clearError("depositAmount");
              }}
              error={errors.depositAmount}
              disabled={saving}
            />
          </div>

          <SelectField
            triggerRef={collectorRef}
            label="Penagih"
            value={collector}
            placeholder={NO_COLLECTOR_LABEL}
            options={collectorOptions(
              candidates.data?.candidates ?? [],
              lease.collector,
            )}
            onValueChange={(next) => {
              setCollector(next);
              clearError("collectorMemberId");
            }}
            hint={disabledCollector ? DISABLED_COLLECTOR_HINT : COLLECTOR_HINT}
            hintTone={disabledCollector ? "warning" : "muted"}
            error={errors.collectorMemberId}
            disabled={saving}
            className="md:[grid-area:collector]"
          />
        </div>
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Batal"
        submitLabel="Simpan sewa"
        saving={saving}
        wide
      />
    </>
  );
}

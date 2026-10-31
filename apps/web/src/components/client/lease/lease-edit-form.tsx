"use client";

import { useId } from "react";
import { useController } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import { useListCollectorCandidates } from "@/lib/api/generated/members/members";
import type { Lease } from "@/lib/api/generated/model";
import { useUpdateLease } from "@/lib/api/generated/tenants/tenants";
import {
  BOOKING_NOTE,
  COLLECTOR_HINT,
  DEPOSIT_HINT,
  DISABLED_COLLECTOR_HINT,
  EDIT_LEASE_NOTE,
  LEASE_TERMS_API_FIELDS,
  NO_COLLECTOR_LABEL,
  changedLeaseBilledFromOptions,
  collectorOptions,
  editStartBox,
  keepsDisabledCollector,
  leaseTermsDefaults,
  leaseTermsFormValues,
  leaseTermsSchema,
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
  const { form, showApiError, alert } = useApiForm({
    schema: leaseTermsSchema(lease),
    defaultValues: leaseTermsDefaults(lease),
    apiFields: LEASE_TERMS_API_FIELDS,
  });
  const { control, clearErrors } = form;
  const { errors } = form.formState;
  // In the form's order, which is the order a failed check focuses them in.
  const { field: start } = useController({ control, name: "startDate" });
  const { field: billing } = useController({ control, name: "billing" });
  const { field: chosenBilledFrom } = useController({
    control,
    name: "billedFrom",
  });
  const { field: deposit } = useController({ control, name: "depositAmount" });
  const { field: collector } = useController({
    control,
    name: "collectorMemberId",
  });

  // The candidates load again whenever the form opens, so a member enabled or disabled since the
  // page cached them shows as they are now.
  const candidates = useListCollectorCandidates(orgId, {
    query: { refetchOnMount: "always" },
  });

  const startDate = start.value;
  const box = editStartBox(startDate, lease.enteredOn, today);
  const options = changedLeaseBilledFromOptions(startDate, lease);
  // The "Mulai tagih dari" date the select shows, as the schema reads it.
  const { billedFrom } = leaseTermsFormValues(
    {
      startDate,
      billing: billing.value,
      billedFrom: chosenBilledFrom.value,
      depositAmount: deposit.value,
      collectorMemberId: collector.value,
    },
    lease,
  );
  const disabledCollector = keepsDisabledCollector(
    collector.value,
    lease.collector,
  );

  const update = useUpdateLease({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = update.isPending;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    update.mutate(
      { orgId, leaseId: lease.id, data: input },
      { onSuccess: onSaved, onError: showApiError },
    );
  });

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}

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
            inputRef={start.ref}
            label="Mulai sewa"
            name="startDate"
            value={startDate}
            onChange={(next) => {
              start.onChange(next);
              clearErrors(["startDate", "billedFrom"]);
            }}
            hint={startDateHint(startDate, lease.termMonths)}
            error={errors.startDate?.message}
            disabled={saving}
            className="md:[grid-area:start]"
          />

          <div className="min-w-0 md:[grid-area:note]">
            <LeaseInfoNote text={EDIT_LEASE_NOTE} />
          </div>

          {box === "billing" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseBillingField
                selectRef={chosenBilledFrom.ref}
                startDate={startDate}
                enteredOn={lease.enteredOn}
                today={today}
                termMonths={lease.termMonths}
                billing={billing.value}
                onBillingChange={(next) => {
                  billing.onChange(next);
                  clearErrors("billedFrom");
                }}
                options={options}
                billedFrom={billedFrom}
                onBilledFromChange={(next) => {
                  chosenBilledFrom.onChange(next);
                  clearErrors("billedFrom");
                }}
                error={errors.billedFrom?.message}
                disabled={saving}
              />
            </div>
          )}
          {box === "booking" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseInfoNote text={BOOKING_NOTE} />
            </div>
          )}
          {box !== "billing" && errors.billedFrom?.message !== undefined && (
            <div className="min-w-0 md:[grid-area:billed]">
              <FieldError
                id={`${id}-billed-error`}
                message={errors.billedFrom.message}
              />
            </div>
          )}

          <div className="min-w-0 md:[grid-area:deposit]">
            <RupiahField
              inputRef={deposit.ref}
              label="Uang jaminan"
              name="depositAmount"
              placeholder="0"
              hint={DEPOSIT_HINT}
              value={deposit.value}
              onChange={(text) => {
                deposit.onChange(text);
                clearErrors("depositAmount");
              }}
              error={errors.depositAmount?.message}
              disabled={saving}
            />
          </div>

          <SelectField
            triggerRef={collector.ref}
            label="Penagih"
            value={collector.value}
            placeholder={NO_COLLECTOR_LABEL}
            options={collectorOptions(
              candidates.data?.candidates ?? [],
              lease.collector,
            )}
            onValueChange={(next) => {
              collector.onChange(next);
              clearErrors("collectorMemberId");
            }}
            hint={disabledCollector ? DISABLED_COLLECTOR_HINT : COLLECTOR_HINT}
            hintTone={disabledCollector ? "warning" : "muted"}
            error={errors.collectorMemberId?.message}
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

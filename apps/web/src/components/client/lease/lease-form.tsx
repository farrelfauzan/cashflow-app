"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { useController } from "react-hook-form";
import { z } from "zod";

import { DateField } from "@/components/client/form/date-field";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import { ApiError } from "@/lib/api/errors";
import { toFormError } from "@/lib/api/form-error";
import {
  getListMembersQueryKey,
  useListCollectorCandidates,
} from "@/lib/api/generated/members/members";
import type { Lease, Tenant } from "@/lib/api/generated/model";
import {
  getGetPropertyQueryKey,
  getListPropertiesQueryKey,
  getListUnitsQueryKey,
  useListProperties,
  useListUnits,
} from "@/lib/api/generated/properties/properties";
import {
  getGetTenantQueryKey,
  getListTenantsQueryKey,
  useCreateLease,
} from "@/lib/api/generated/tenants/tenants";
import type { LeaseFormContext } from "./type";
import {
  BOOKING_NOTE,
  COLLECTOR_HINT,
  DEPOSIT_HINT,
  FREE_UNITS_HINT,
  LEASE_API_FIELDS,
  LEASE_FORM_DEFAULTS,
  NO_COLLECTOR_LABEL,
  NO_FREE_UNIT_TEXT,
  NO_PROPERTY_TEXT,
  collectorOptions,
  freeUnits,
  isCalendarDate,
  leaseFormSchema,
  leaseFormValues,
  newLeaseBilledFromOptions,
  startDateHint,
  startTiming,
  TERM_NO_UNIT_PLACEHOLDER,
  TERM_PLACEHOLDER,
  termFromValue,
  termHint,
  termOptions,
} from "./utils";

import { LeaseBillingField } from "./lease-billing-field";
import { LeaseInfoNote } from "./lease-info-note";
import { LeasePanelFooter } from "./lease-panel-footer";

type LeaseFormProps = {
  orgId: string;
  tenant: Pick<Tenant, "id" | "name">;
  // "Today" in Jakarta, read once when the panel opened.
  today: string;
  onSaved: (lease: Lease) => void;
};

// "Sewakan unit" (boards Sewa-Baru.dc.html and Sewa-Baru-Phone.dc.html): the property and one of
// its free units, "Masa sewa" (one of the unit's terms, with its price), the start date, "Mulai
// tagih dari" for a start before today, the deposit, and the collector. On the phone the fields
// stack in that order; on iPad "Masa sewa" takes the left half's width so its price fits, and the
// billing choice has the right half of the wide panel to itself, so it stays in view. The save button sits in the
// panel's footer and submits the form through its `form` attribute.
export function LeaseForm({ orgId, tenant, today, onSaved }: LeaseFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();

  // The schema checks what the form shows: the property (the only one is chosen already) and
  // the unit while it is still free. Both come from the fields and the lists loaded below, so the
  // context is set further down this render; the schema reads it on submit, never while
  // rendering.
  let context: LeaseFormContext = {
    tenantId: tenant.id,
    today,
    propertyId: "",
    unit: undefined,
  };
  const { form, showApiError, alert } = useApiForm({
    schema: z.lazy(() => leaseFormSchema(context)),
    defaultValues: LEASE_FORM_DEFAULTS,
    apiFields: LEASE_API_FIELDS,
  });
  const { control, clearErrors, setValue } = form;
  const { errors } = form.formState;
  // In the form's order, which is the order a failed check focuses them in.
  const { field: chosenProperty } = useController({
    control,
    name: "propertyId",
  });
  const { field: chosenUnit } = useController({ control, name: "unitId" });
  const { field: chosenTerm } = useController({ control, name: "termMonths" });
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

  // The choices load again whenever the form opens, so a property, unit or member added (or a
  // unit taken) since the list screen cached them shows as it is now.
  const fresh = { query: { refetchOnMount: "always" as const } };

  // The properties the member may see; with only one, it is chosen already.
  const properties = useListProperties(orgId, { pageSize: 100 }, fresh);
  const propertyItems = properties.data?.items ?? [];
  const propertyId =
    chosenProperty.value !== ""
      ? chosenProperty.value
      : propertyItems.length === 1
        ? (propertyItems[0]?.id ?? "")
        : "";

  const units = useListUnits(
    orgId,
    propertyId,
    { pageSize: 100 },
    { query: { ...fresh.query, enabled: propertyId !== "" } },
  );
  const free = freeUnits(units.data?.items ?? []);
  const unit = free.find((candidate) => candidate.id === chosenUnit.value);
  context = { tenantId: tenant.id, today, propertyId, unit };
  // The term and the "Mulai tagih dari" date the selects show, as the schema reads them.
  const shown = leaseFormValues(
    {
      propertyId,
      unitId: chosenUnit.value,
      termMonths: chosenTerm.value,
      startDate: start.value,
      billing: billing.value,
      billedFrom: chosenBilledFrom.value,
      depositAmount: deposit.value,
      collectorMemberId: collector.value,
    },
    context,
  );
  const termMonths = shown.termMonths;
  // The billing words and choices follow the term; monthly until a priced term is chosen.
  const term = termMonths ?? 1;
  const termNote = termHint(unit, termMonths);

  const candidates = useListCollectorCandidates(orgId, fresh);

  const startDate = start.value;
  const timing = isCalendarDate(startDate)
    ? startTiming(startDate, today)
    : null;
  const options = newLeaseBilledFromOptions(startDate, today, term);

  const create = useCreateLease({
    mutation: {
      // A new lease changes the tenant, the tenant list, the unit's and property's occupancy, and
      // its collector's lease count in the member list.
      onSuccess: () =>
        Promise.all(
          [
            getGetTenantQueryKey(orgId, tenant.id),
            getListTenantsQueryKey(orgId),
            getListUnitsQueryKey(orgId, propertyId),
            getListPropertiesQueryKey(orgId),
            getGetPropertyQueryKey(orgId, propertyId),
            getListMembersQueryKey(orgId),
          ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
        ),
    },
  });
  const saving = create.isPending;

  function handleError(error: unknown) {
    // The unit was taken since the list loaded: reload it, so that unit leaves the choices and
    // the select goes back to "Pilih unit" with the refusal still under it.
    if (error instanceof ApiError && error.code === "unit_occupied") {
      void queryClient.invalidateQueries({
        queryKey: getListUnitsQueryKey(orgId, propertyId),
      });
    }
    showApiError(error);
  }

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    create.mutate(
      { orgId, data: input },
      { onSuccess: onSaved, onError: handleError },
    );
  });

  const unitHint =
    propertyId !== "" && units.data !== undefined && free.length === 0
      ? NO_FREE_UNIT_TEXT
      : FREE_UNITS_HINT;

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}
        {properties.isError && (
          <FormAlert message={toFormError(properties.error, []).message} />
        )}

        <div className="flex flex-col gap-[18px] md:grid md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,2fr)] md:grid-rows-[auto_auto_auto_auto_auto_1fr] md:items-start md:gap-x-4 md:[grid-template-areas:'prop_unit_billed'_'term_term_billed'_'deposit_deposit_billed'_'start_start_billed'_'collector_collector_billed'_'._._billed']">
          <SelectField
            triggerRef={chosenProperty.ref}
            label="Properti"
            value={propertyId}
            placeholder="Pilih properti"
            options={propertyItems.map((p) => ({ value: p.id, label: p.name }))}
            onValueChange={(next) => {
              chosenProperty.onChange(next);
              setValue("unitId", "");
              setValue("termMonths", null);
              clearErrors(["propertyId", "unitId", "termMonths"]);
            }}
            hint={
              properties.data !== undefined && propertyItems.length === 0
                ? NO_PROPERTY_TEXT
                : null
            }
            error={errors.propertyId?.message}
            disabled={saving}
            className="md:[grid-area:prop]"
          />

          <SelectField
            triggerRef={chosenUnit.ref}
            label="Unit"
            value={unit === undefined ? "" : chosenUnit.value}
            placeholder="Pilih unit"
            options={free.map((u) => ({ value: u.id, label: u.label }))}
            onValueChange={(next) => {
              chosenUnit.onChange(next);
              setValue("termMonths", null);
              clearErrors(["unitId", "termMonths", "billedFrom"]);
            }}
            hint={unitHint}
            error={errors.unitId?.message}
            disabled={saving || propertyId === ""}
            className="md:[grid-area:unit]"
          />

          <SelectField
            triggerRef={chosenTerm.ref}
            label="Masa sewa"
            value={termMonths === null ? "" : String(termMonths)}
            placeholder={
              unit === undefined ? TERM_NO_UNIT_PLACEHOLDER : TERM_PLACEHOLDER
            }
            options={termOptions(unit)}
            onValueChange={(next) => {
              chosenTerm.onChange(termFromValue(unit, next));
              clearErrors(["termMonths", "billedFrom"]);
            }}
            // After "Simpan sewa" the same message is the field's error; it shows once.
            hint={
              errors.termMonths?.message === termNote.text
                ? null
                : termNote.text
            }
            hintTone={termNote.tone}
            error={errors.termMonths?.message}
            disabled={saving || unit === undefined}
            className="md:[grid-area:term]"
          />

          <DateField
            inputRef={start.ref}
            label="Mulai sewa"
            name="startDate"
            value={startDate}
            onChange={(next) => {
              start.onChange(next);
              clearErrors(["startDate", "billedFrom"]);
            }}
            hint={startDateHint(startDate, term)}
            error={errors.startDate?.message}
            disabled={saving}
            className="md:[grid-area:start]"
          />

          {timing === "past" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseBillingField
                selectRef={chosenBilledFrom.ref}
                startDate={startDate}
                enteredOn={today}
                today={today}
                termMonths={term}
                billing={billing.value}
                onBillingChange={(next) => {
                  billing.onChange(next);
                  clearErrors("billedFrom");
                }}
                options={options}
                billedFrom={shown.billedFrom}
                onBilledFromChange={(next) => {
                  chosenBilledFrom.onChange(next);
                  clearErrors("billedFrom");
                }}
                error={errors.billedFrom?.message}
                disabled={saving}
              />
            </div>
          )}
          {timing === "future" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseInfoNote text={BOOKING_NOTE} />
            </div>
          )}
          {timing !== "past" && errors.billedFrom?.message !== undefined && (
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
              undefined,
            )}
            onValueChange={(next) => {
              collector.onChange(next);
              clearErrors("collectorMemberId");
            }}
            hint={COLLECTOR_HINT}
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

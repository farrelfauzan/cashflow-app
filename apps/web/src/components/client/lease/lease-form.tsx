"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { DateField } from "@/components/client/form/date-field";
import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { ApiError } from "@/lib/api/errors";
import { toFormError } from "@/lib/api/form-error";
import { useListCollectorCandidates } from "@/lib/api/generated/members/members";
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
import type { BillingChoice, LeaseFormErrors, LeaseFormField } from "./type";
import {
  BOOKING_NOTE,
  COLLECTOR_HINT,
  DEPOSIT_HINT,
  FREE_UNITS_HINT,
  LEASE_API_FIELDS,
  NO_COLLECTOR,
  NO_COLLECTOR_LABEL,
  NO_FREE_UNIT_TEXT,
  NO_PROPERTY_TEXT,
  billedFromOptions,
  checkLeaseForm,
  collectorOptions,
  defaultBilledFrom,
  firstLeaseError,
  freeUnits,
  isCalendarDate,
  isLeaseFormField,
  rentSuggestion,
  rentTextFor,
  startDateHint,
  startTiming,
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
// its free units, the start date, "Mulai tagih dari" for a start before today, the rent and
// deposit, and the collector. On the phone the fields stack in that order; on iPad the billing
// choice has the right half of the wide panel to itself, so it stays in view. The save button
// sits in the panel's footer and submits the form through its `form` attribute.
export function LeaseForm({ orgId, tenant, today, onSaved }: LeaseFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const propertyRef = useRef<HTMLButtonElement>(null);
  const unitRef = useRef<HTMLButtonElement>(null);
  const startRef = useRef<HTMLInputElement>(null);
  const billedFromRef = useRef<HTMLButtonElement>(null);
  const rentRef = useRef<HTMLInputElement>(null);
  const depositRef = useRef<HTMLInputElement>(null);
  const collectorRef = useRef<HTMLButtonElement>(null);

  const [chosenProperty, setChosenProperty] = useState("");
  const [unitId, setUnitId] = useState("");
  const [startDate, setStartDate] = useState("");
  const [billing, setBilling] = useState<BillingChoice>("start");
  const [chosenBilledFrom, setChosenBilledFrom] = useState("");
  const [rent, setRent] = useState("");
  const [deposit, setDeposit] = useState("");
  const [collector, setCollector] = useState(NO_COLLECTOR);
  const [errors, setErrors] = useState<LeaseFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

  // The choices load again whenever the form opens, so a property, unit or member added (or a
  // unit taken) since the list screen cached them shows as it is now.
  const fresh = { query: { refetchOnMount: "always" as const } };

  // The properties the member may see; with only one, it is chosen already.
  const properties = useListProperties(orgId, { pageSize: 100 }, fresh);
  const propertyItems = properties.data?.items ?? [];
  const propertyId =
    chosenProperty !== ""
      ? chosenProperty
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
  const unit = free.find((candidate) => candidate.id === unitId);

  const candidates = useListCollectorCandidates(orgId, fresh);

  const timing = isCalendarDate(startDate)
    ? startTiming(startDate, today)
    : null;
  const options = timing === "past" ? billedFromOptions(startDate, today) : [];
  const billedFrom = options.some((o) => o.value === chosenBilledFrom)
    ? chosenBilledFrom
    : defaultBilledFrom(options, today);

  const create = useCreateLease({
    mutation: {
      // A new lease changes the tenant, the tenant list and the unit's and property's occupancy.
      onSuccess: () =>
        Promise.all(
          [
            getGetTenantQueryKey(orgId, tenant.id),
            getListTenantsQueryKey(orgId),
            getListUnitsQueryKey(orgId, propertyId),
            getListPropertiesQueryKey(orgId),
            getGetPropertyQueryKey(orgId, propertyId),
          ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
        ),
    },
  });
  const saving = create.isPending;

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
      propertyId: propertyRef,
      unitId: unitRef,
      startDate: startRef,
      billedFrom: billedFromRef,
      rentAmount: rentRef,
      depositAmount: depositRef,
      collectorMemberId: collectorRef,
    };
    if (focusTarget.field !== null) ref[focusTarget.field].current?.focus();
  }, [focusTarget, saving]);

  function showErrors(found: LeaseFormErrors) {
    setErrors(found);
    setFocusTarget({ field: firstLeaseError(found) });
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, LEASE_API_FIELDS);
    // The unit was taken since the list loaded: reload it, so that unit leaves the choices and
    // the select goes back to "Pilih unit" with the refusal still under it.
    if (error instanceof ApiError && error.code === "unit_occupied") {
      void queryClient.invalidateQueries({
        queryKey: getListUnitsQueryKey(orgId, propertyId),
      });
    }
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
    const checked = checkLeaseForm(
      {
        tenantId: tenant.id,
        unitId: unit === undefined ? "" : unitId,
        startDate,
        billing,
        billedFrom,
        rent,
        deposit,
        collector,
      },
      propertyId,
      today,
    );
    if (!checked.ok) {
      showErrors(checked.errors);
      return;
    }
    setErrors({});
    create.mutate(
      { orgId, data: checked.input },
      { onSuccess: onSaved, onError: handleError },
    );
  }

  const unitHint =
    propertyId !== "" && units.data !== undefined && free.length === 0
      ? NO_FREE_UNIT_TEXT
      : FREE_UNITS_HINT;

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={handleSubmit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== null && <FormAlert message={alert} />}
        {properties.isError && (
          <FormAlert message={toFormError(properties.error, []).message} />
        )}

        <div className="flex flex-col gap-[18px] md:grid md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,2fr)] md:grid-rows-[auto_auto_auto_auto_1fr] md:items-start md:gap-x-4 md:[grid-template-areas:'prop_unit_billed'_'rent_deposit_billed'_'start_start_billed'_'collector_collector_billed'_'._._billed']">
          <SelectField
            triggerRef={propertyRef}
            label="Properti"
            value={propertyId}
            placeholder="Pilih properti"
            options={propertyItems.map((p) => ({ value: p.id, label: p.name }))}
            onValueChange={(next) => {
              setChosenProperty(next);
              setUnitId("");
              clearError("propertyId");
              clearError("unitId");
            }}
            hint={
              properties.data !== undefined && propertyItems.length === 0
                ? NO_PROPERTY_TEXT
                : null
            }
            error={errors.propertyId}
            disabled={saving}
            className="md:[grid-area:prop]"
          />

          <SelectField
            triggerRef={unitRef}
            label="Unit"
            value={unit === undefined ? "" : unitId}
            placeholder="Pilih unit"
            options={free.map((u) => ({ value: u.id, label: u.label }))}
            onValueChange={(next) => {
              setUnitId(next);
              const picked = free.find((candidate) => candidate.id === next);
              if (picked?.defaultRent !== undefined) {
                setRent(rentTextFor(picked));
                clearError("rentAmount");
              }
              clearError("unitId");
            }}
            hint={unitHint}
            error={errors.unitId}
            disabled={saving || propertyId === ""}
            className="md:[grid-area:unit]"
          />

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

          {timing === "past" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseBillingField
                selectRef={billedFromRef}
                startDate={startDate}
                enteredOn={today}
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
          {timing === "future" && (
            <div className="min-w-0 md:[grid-area:billed]">
              <LeaseInfoNote text={BOOKING_NOTE} />
            </div>
          )}
          {timing !== "past" && errors.billedFrom !== undefined && (
            <div className="min-w-0 md:[grid-area:billed]">
              <FieldError
                id={`${id}-billed-error`}
                message={errors.billedFrom}
              />
            </div>
          )}

          <div className="min-w-0 md:[grid-area:rent]">
            <RupiahField
              inputRef={rentRef}
              label="Sewa per bulan"
              name="rentAmount"
              placeholder="1.250.000"
              hint={rentSuggestion(unit)}
              value={rent}
              onChange={(text) => {
                setRent(text);
                clearError("rentAmount");
              }}
              error={errors.rentAmount}
              disabled={saving}
            />
          </div>

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
              undefined,
            )}
            onValueChange={(next) => {
              setCollector(next);
              clearError("collectorMemberId");
            }}
            hint={COLLECTOR_HINT}
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

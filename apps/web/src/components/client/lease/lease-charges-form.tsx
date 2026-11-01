"use client";

import { Plus } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { flushSync } from "react-dom";

import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { FIELD_HINT_CLASS } from "@/components/client/form/utils";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import type { ChargeType, Lease } from "@/lib/api/generated/model";
import {
  useListChargeTypes,
  useSetLeaseCharges,
} from "@/lib/api/generated/tenants/tenants";
import {
  ADD_CHARGE_TYPE_LABEL,
  chargeEntriesFromLease,
  chargeField,
  chargeFields,
  chargeRows,
  chargesTermHint,
  LEASE_CHARGES_INTRO,
  chargeRowApiError,
  leaseChargesFormSchema,
  LOADING_CHARGE_TYPES_TEXT,
  METERED_CHARGES_NOTE,
  meteredChargeTypeAddedText,
  NO_FIXED_CHARGE_TYPES_TEXT,
  registerInOrder,
  withChargeChecked,
} from "./utils";

import { LeaseChargeRow } from "./lease-charge-row";
import { LeaseInfoNote } from "./lease-info-note";
import { LeasePanelFooter } from "./lease-panel-footer";
import { NewChargeTypeBox } from "./new-charge-type-box";
import { useRefreshLease } from "./use-refresh-lease";

type LeaseChargesFormProps = {
  orgId: string;
  lease: Lease;
  // "Tambah jenis biaya baru": lease:manage for the whole organization.
  canAddChargeType: boolean;
  onSaved: (lease: Lease) => void;
};

// "Biaya tambahan" (boards Sewa-Biaya.dc.html and Sewa-Biaya-Phone.dc.html): the lease's fixed
// monthly extras, which a lease on a longer term bills times its months (said under the intro).
// One row per non-metered charge type, loaded fresh each time the panel opens; the lease's charges
// start checked. Saving sends the whole set, so a row left unchecked is removed. A member who may
// manage leases everywhere can add a charge type here.
export function LeaseChargesForm({
  orgId,
  lease,
  canAddChargeType,
  onSaved,
}: LeaseChargesFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshLease(orgId);

  const types = useListChargeTypes(orgId, {
    query: { refetchOnMount: "always" },
  });
  const chargeTypes = types.data?.chargeTypes ?? [];
  const { form, showApiError, alert } = useApiForm({
    schema: leaseChargesFormSchema(chargeTypes),
    defaultValues: { charges: chargeEntriesFromLease(lease.charges) },
    apiFields: chargeFields(chargeTypes),
  });
  const [boxOpen, setBoxOpen] = useState(false);
  const [meteredAdded, setMeteredAdded] = useState<string | null>(null);
  const termHint = chargesTermHint(lease.termMonths);

  const save = useSetLeaseCharges({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = save.isPending;

  const rows = chargeRows(chargeTypes, {});

  // A type just added is checked and its amount focused. The list of types is already loaded
  // again, so rendering now shows its row, and its amount can take the focus.
  function handleAdded(type: ChargeType) {
    if (type.metered) {
      setBoxOpen(false);
      setMeteredAdded(meteredChargeTypeAddedText(type.name));
      return;
    }
    const name = chargeField(type.id);
    flushSync(() => {
      setBoxOpen(false);
      setMeteredAdded(null);
      form.setValue(name, withChargeChecked(form.getValues(name), true));
    });
    form.setFocus(name);
  }

  const check = form.handleSubmit((input) => {
    // Without the types the rows are unknown, and sending none would remove every extra.
    if (saving || types.data === undefined) return;
    save.mutate(
      { orgId, leaseId: lease.id, data: input },
      {
        onSuccess: onSaved,
        // An error on charges[i] goes under the row sent at index i.
        onError: (failure, variables) =>
          showApiError(chargeRowApiError(failure, variables.data)),
      },
    );
  });

  // A failed check focuses the first bad row on screen (registerInOrder).
  function submit(event: FormEvent<HTMLFormElement>) {
    registerInOrder(form, chargeFields(chargeTypes));
    return check(event);
  }

  let list;
  if (types.isError && types.data === undefined) {
    list = <FormAlert message={toFormError(types.error, []).message} />;
  } else if (types.data === undefined) {
    list = (
      <p className="text-base text-muted-foreground">
        {LOADING_CHARGE_TYPES_TEXT}
      </p>
    );
  } else if (rows.length === 0) {
    list = (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {NO_FIXED_CHARGE_TYPES_TEXT}
      </p>
    );
  } else {
    list = (
      <div className="flex flex-col">
        {rows.map((row) => (
          <LeaseChargeRow
            key={row.chargeTypeId}
            form={form}
            chargeTypeId={row.chargeTypeId}
            name={row.name}
            disabled={saving}
          />
        ))}
      </div>
    );
  }

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}
        <p className="text-[15px] leading-[1.55] md:text-base">
          {LEASE_CHARGES_INTRO}
        </p>
        {termHint !== null && <p className={FIELD_HINT_CLASS}>{termHint}</p>}
        {list}
        <LeaseInfoNote text={METERED_CHARGES_NOTE} />
        {meteredAdded !== null && (
          <p
            role="status"
            className="text-[15px] leading-normal font-semibold text-primary md:text-base"
          >
            {meteredAdded}
          </p>
        )}
        {canAddChargeType && !boxOpen && (
          <Button
            type="button"
            variant="ghost"
            disabled={saving}
            onClick={() => {
              setMeteredAdded(null);
              setBoxOpen(true);
            }}
            className="h-11 self-start rounded-xl px-1 text-base font-semibold text-primary hover:bg-accent hover:text-primary"
          >
            <Plus aria-hidden="true" className="size-5" strokeWidth={2.2} />
            {ADD_CHARGE_TYPE_LABEL}
          </Button>
        )}
        {canAddChargeType && boxOpen && (
          <NewChargeTypeBox
            orgId={orgId}
            disabled={saving}
            onAdded={handleAdded}
          />
        )}
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Batal"
        submitLabel="Simpan biaya tambahan"
        saving={saving}
        wide={false}
      />
    </>
  );
}

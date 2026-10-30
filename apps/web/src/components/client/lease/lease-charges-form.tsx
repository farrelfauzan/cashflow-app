"use client";

import { Plus } from "lucide-react";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { FIELD_HINT_CLASS } from "@/components/client/form/utils";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import type { ChargeType, Lease } from "@/lib/api/generated/model";
import {
  useListChargeTypes,
  useSetLeaseCharges,
} from "@/lib/api/generated/tenants/tenants";
import type { ChargeEntries, ChargeErrors } from "./type";
import {
  ADD_CHARGE_TYPE_LABEL,
  chargeAmountId,
  chargeApiFields,
  chargeEntriesFromLease,
  chargeRowForApiField,
  chargeRows,
  chargesTermHint,
  checkLeaseCharges,
  LEASE_CHARGES_INTRO,
  LOADING_CHARGE_TYPES_TEXT,
  METERED_CHARGES_NOTE,
  meteredChargeTypeAddedText,
  NO_FIXED_CHARGE_TYPES_TEXT,
  withChargeAmount,
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
  const [entries, setEntries] = useState<ChargeEntries>(() =>
    chargeEntriesFromLease(lease.charges),
  );
  const [errors, setErrors] = useState<ChargeErrors>({});
  const [alert, setAlert] = useState<string | null>(null);
  const [boxOpen, setBoxOpen] = useState(false);
  const [meteredAdded, setMeteredAdded] = useState<string | null>(null);
  const termHint = chargesTermHint(lease.termMonths);

  const save = useSetLeaseCharges({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = save.isPending;

  const rows =
    types.data === undefined ? [] : chargeRows(types.data.chargeTypes, entries);

  // Focus a row's amount: the first bad one after a refused save, or a type just added. Each
  // request is handled once, when the row is there and the form is enabled.
  const [focusRow, setFocusRow] = useState<{ id: string } | null>(null);
  const focused = useRef<object | null>(null);
  useEffect(() => {
    if (focusRow === null || saving || focused.current === focusRow) return;
    const input = document.getElementById(chargeAmountId(id, focusRow.id));
    if (input === null) return;
    focused.current = focusRow;
    input.focus();
    // types.data: a type just added has its row once the list is loaded again.
  }, [focusRow, saving, types.data, id]);

  function changeRow(chargeTypeId: string, next: ChargeEntries) {
    setEntries(next);
    setErrors((current) => ({ ...current, [chargeTypeId]: undefined }));
  }

  function handleAdded(type: ChargeType) {
    setBoxOpen(false);
    if (type.metered) {
      setMeteredAdded(meteredChargeTypeAddedText(type.name));
      return;
    }
    setMeteredAdded(null);
    setEntries((current) => withChargeChecked(current, type.id, true));
    setFocusRow({ id: type.id });
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    // Without the types the rows are unknown, and sending none would remove every extra.
    if (saving || types.data === undefined) return;
    const checked = checkLeaseCharges(rows);
    if (!checked.ok) {
      setErrors(checked.errors);
      setFocusRow({ id: checked.first });
      return;
    }
    setErrors({});
    setAlert(null);
    const { sent } = checked;
    save.mutate(
      { orgId, leaseId: lease.id, data: checked.input },
      {
        onSuccess: onSaved,
        onError: (failure) => {
          const failed = toFormError(failure, chargeApiFields(sent));
          const row = chargeRowForApiField(failed.field, sent);
          if (row === null) {
            setAlert(failed.message);
            return;
          }
          setErrors({ [row]: failed.message });
          setFocusRow({ id: row });
        },
      },
    );
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
            amountId={chargeAmountId(id, row.chargeTypeId)}
            row={row}
            error={errors[row.chargeTypeId]}
            disabled={saving}
            onCheckedChange={(checked) =>
              changeRow(
                row.chargeTypeId,
                withChargeChecked(entries, row.chargeTypeId, checked),
              )
            }
            onAmountChange={(text) =>
              changeRow(
                row.chargeTypeId,
                withChargeAmount(entries, row.chargeTypeId, text),
              )
            }
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
        onSubmit={handleSubmit}
        className="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-6 py-5"
      >
        {alert !== null && <FormAlert message={alert} />}
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

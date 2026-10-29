"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { toFormError } from "@/lib/api/form-error";
import type { Lease } from "@/lib/api/generated/model";
import { useMoveOut } from "@/lib/api/generated/tenants/tenants";
import {
  MOVE_OUT_API_FIELDS,
  MOVE_OUT_EFFECTS,
  checkMoveOut,
  moveOutHint,
} from "./utils";

import { LeaseEffectsBox } from "./lease-effects-box";
import { LeasePanelFooter } from "./lease-panel-footer";
import { useRefreshLease } from "./use-refresh-lease";

type MoveOutFormProps = {
  orgId: string;
  // A lease that has started.
  lease: Lease;
  // "Today" in Jakarta, read once when the page opened.
  today: string;
  onSaved: (lease: Lease) => void;
};

// "Catat pindah keluar" (boards Sewa-Pindah.dc.html and Sewa-Pindah-Phone.dc.html): the lease's
// last day, today unless changed, and what ending it does. The unit is free again right away.
export function MoveOutForm({
  orgId,
  lease,
  today,
  onSaved,
}: MoveOutFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshLease(orgId);
  const dateRef = useRef<HTMLInputElement>(null);
  const [date, setDate] = useState(today);
  const [error, setError] = useState<string | undefined>(undefined);
  const [alert, setAlert] = useState<string | null>(null);

  const moveOut = useMoveOut({
    mutation: { onSuccess: (ended) => refresh(ended, true) },
  });
  const saving = moveOut.isPending;

  // Focus the date after a refused save, once the field is enabled again.
  const [focusDate, setFocusDate] = useState<object | null>(null);
  useEffect(() => {
    if (focusDate !== null && !saving) dateRef.current?.focus();
  }, [focusDate, saving]);

  function showError(message: string) {
    setError(message);
    setFocusDate({});
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    setAlert(null);
    const checked = checkMoveOut(date, lease.startDate, today);
    if (!checked.ok) {
      showError(checked.error);
      return;
    }
    setError(undefined);
    moveOut.mutate(
      { orgId, leaseId: lease.id, data: { endDate: checked.date } },
      {
        onSuccess: onSaved,
        onError: (failure) => {
          const failed = toFormError(failure, MOVE_OUT_API_FIELDS);
          if (failed.field !== null) showError(failed.message);
          else setAlert(failed.message);
        },
      },
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
        <DateField
          inputRef={dateRef}
          label="Hari terakhir menyewa"
          name="endDate"
          value={date}
          onChange={(next) => {
            setDate(next);
            setError(undefined);
          }}
          hint={moveOutHint(lease.startDate)}
          error={error}
          disabled={saving}
        />
        <LeaseEffectsBox items={MOVE_OUT_EFFECTS} />
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Batal"
        submitLabel="Catat pindah keluar"
        saving={saving}
        wide={false}
      />
    </>
  );
}

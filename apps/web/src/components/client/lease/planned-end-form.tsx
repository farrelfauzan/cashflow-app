"use client";

import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import type { Lease } from "@/lib/api/generated/model";
import { useSetLeasePlannedEnd } from "@/lib/api/generated/tenants/tenants";
import {
  PLANNED_END_API_FIELDS,
  checkPlannedEnd,
  currentPlannedEndText,
  plannedEndEffects,
  plannedEndHint,
  plannedEndSuggestion,
} from "./utils";

import { LeaseEffectsBox } from "./lease-effects-box";
import { LeasePanelFooter } from "./lease-panel-footer";
import { useRefreshLease } from "./use-refresh-lease";

type PlannedEndFormProps = {
  orgId: string;
  lease: Lease;
  // "Today" in Jakarta, read once when the page opened.
  today: string;
  // The saved lease: plannedEndDate is missing after "Hapus rencana keluar".
  onSaved: (lease: Lease) => void;
};

// "Rencana keluar" (boards Sewa-Rencana.dc.html and Sewa-Rencana-Phone.dc.html): the tenant's
// planned last day, suggested as the day before the next period of its term starts, what that
// does, and, when one is set, "Hapus rencana keluar". The API also needs correction:create when
// the date cancels an invoice; its refusal shows in the alert.
export function PlannedEndForm({
  orgId,
  lease,
  today,
  onSaved,
}: PlannedEndFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshLease(orgId);
  const dateRef = useRef<HTMLInputElement>(null);
  const [suggestion] = useState(() =>
    plannedEndSuggestion(lease.startDate, today, lease.termMonths),
  );
  const [date, setDate] = useState(
    () => lease.plannedEndDate ?? suggestion.date,
  );
  const [error, setError] = useState<string | undefined>(undefined);
  const [alert, setAlert] = useState<string | null>(null);

  const save = useSetLeasePlannedEnd({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = save.isPending;

  // Focus the date after a refused save, once the field is enabled again.
  const [focusDate, setFocusDate] = useState<object | null>(null);
  useEffect(() => {
    if (focusDate !== null && !saving) dateRef.current?.focus();
  }, [focusDate, saving]);

  function showError(message: string) {
    setError(message);
    setFocusDate({});
  }

  function send(plannedEndDate: string | null) {
    setAlert(null);
    save.mutate(
      { orgId, leaseId: lease.id, data: { plannedEndDate } },
      {
        onSuccess: onSaved,
        onError: (failure) => {
          const failed = toFormError(failure, PLANNED_END_API_FIELDS);
          if (failed.field !== null) showError(failed.message);
          else setAlert(failed.message);
        },
      },
    );
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;
    const checked = checkPlannedEnd(date, lease.startDate, today);
    if (!checked.ok) {
      showError(checked.error);
      return;
    }
    setError(undefined);
    send(checked.date);
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
          label="Hari terakhir penyewa tinggal"
          name="plannedEndDate"
          value={date}
          onChange={(next) => {
            setDate(next);
            setError(undefined);
          }}
          hint={plannedEndHint(suggestion, lease.termMonths)}
          error={error}
          disabled={saving}
        />
        <LeaseEffectsBox items={plannedEndEffects(lease.termMonths)} />
        {lease.plannedEndDate !== undefined && (
          <div className="flex flex-col gap-2 border-t pt-4">
            <p className="text-sm leading-normal text-muted-foreground md:text-base">
              {currentPlannedEndText(lease.plannedEndDate)}
            </p>
            <Button
              type="button"
              variant="outline"
              disabled={saving}
              onClick={() => {
                if (!saving) send(null);
              }}
              className="h-11 self-start rounded-xl border-[1.5px] border-destructive bg-card px-[18px] text-[15px] font-semibold text-destructive hover:bg-destructive/10 hover:text-destructive md:text-base"
            >
              Hapus rencana keluar
            </Button>
          </div>
        )}
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Batal"
        submitLabel="Simpan rencana keluar"
        saving={saving}
        wide={false}
      />
    </>
  );
}

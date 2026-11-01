"use client";

import { useId, useState } from "react";
import { Controller } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import type { Lease } from "@/lib/api/generated/model";
import { useSetLeasePlannedEnd } from "@/lib/api/generated/tenants/tenants";
import {
  PLANNED_END_API_FIELDS,
  currentPlannedEndText,
  plannedEndEffects,
  plannedEndFormSchema,
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
  const [suggestion] = useState(() =>
    plannedEndSuggestion(lease.startDate, today, lease.termMonths),
  );
  const { form, showApiError, alert } = useApiForm({
    schema: plannedEndFormSchema(lease.startDate, today),
    defaultValues: { plannedEndDate: lease.plannedEndDate ?? suggestion.date },
    apiFields: PLANNED_END_API_FIELDS,
  });

  const save = useSetLeasePlannedEnd({
    mutation: { onSuccess: (saved) => refresh(saved, false) },
  });
  const saving = save.isPending;

  function send(plannedEndDate: string | null) {
    save.mutate(
      { orgId, leaseId: lease.id, data: { plannedEndDate } },
      { onSuccess: onSaved, onError: showApiError },
    );
  }

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    send(input.plannedEndDate);
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
        <Controller
          control={form.control}
          name="plannedEndDate"
          render={({ field, fieldState }) => (
            <DateField
              inputRef={field.ref}
              label="Hari terakhir penyewa tinggal"
              name="plannedEndDate"
              value={field.value}
              onChange={(date) => {
                field.onChange(date);
                form.clearErrors("plannedEndDate");
              }}
              hint={plannedEndHint(suggestion, lease.termMonths)}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
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
                if (saving) return;
                // Sent without the date's check; only the alert of an earlier refusal goes.
                form.clearErrors("root.server");
                send(null);
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

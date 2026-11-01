"use client";

import { useId } from "react";
import { Controller } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import type { Lease } from "@/lib/api/generated/model";
import { useMoveOut } from "@/lib/api/generated/tenants/tenants";
import {
  MOVE_OUT_API_FIELDS,
  moveOutEffects,
  moveOutFormSchema,
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
  const { form, showApiError, alert } = useApiForm({
    schema: moveOutFormSchema(lease.startDate, today),
    defaultValues: { endDate: today },
    apiFields: MOVE_OUT_API_FIELDS,
  });

  const moveOut = useMoveOut({
    mutation: { onSuccess: (ended) => refresh(ended, true) },
  });
  const saving = moveOut.isPending;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    moveOut.mutate(
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
        <Controller
          control={form.control}
          name="endDate"
          render={({ field, fieldState }) => (
            <DateField
              inputRef={field.ref}
              label="Hari terakhir menyewa"
              name="endDate"
              value={field.value}
              onChange={(date) => {
                field.onChange(date);
                form.clearErrors("endDate");
              }}
              hint={moveOutHint(lease.startDate)}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />
        <LeaseEffectsBox items={moveOutEffects(lease.termMonths)} />
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

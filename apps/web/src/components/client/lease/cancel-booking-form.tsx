"use client";

import { useId } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
} from "@/components/client/form/utils";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { Lease } from "@/lib/api/generated/model";
import { useMoveOut } from "@/lib/api/generated/tenants/tenants";
import { formatDate } from "@/lib/format/date";
import { cn } from "@/lib/utils";
import {
  CANCEL_BOOKING_API_FIELDS,
  CANCEL_BOOKING_HINT,
  cancelBookingFormSchema,
  cancelBookingText,
} from "./utils";

import { LeaseInfoNote } from "./lease-info-note";
import { LeasePanelFooter } from "./lease-panel-footer";
import { useRefreshLease } from "./use-refresh-lease";

type CancelBookingFormProps = {
  orgId: string;
  // A booking: a lease whose start date is after today.
  lease: Lease;
  onSaved: (lease: Lease) => void;
};

// "Batalkan sewa" (boards Sewa-Batal.dc.html and Sewa-Batal-Phone.dc.html): a booking is cancelled
// by ending it on its own start date, the only date the API accepts for it, so the date is shown
// and not asked. It is never invoiced, and the unit is free again right away.
export function CancelBookingForm({
  orgId,
  lease,
  onSaved,
}: CancelBookingFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const refresh = useRefreshLease(orgId);
  const { form, showApiError, alert } = useApiForm({
    schema: cancelBookingFormSchema,
    defaultValues: {},
    apiFields: CANCEL_BOOKING_API_FIELDS,
  });

  const moveOut = useMoveOut({
    mutation: { onSuccess: (ended) => refresh(ended, true) },
  });
  const saving = moveOut.isPending;

  const submit = form.handleSubmit(() => {
    if (saving) return;
    moveOut.mutate(
      { orgId, leaseId: lease.id, data: { endDate: lease.startDate } },
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
        <LeaseInfoNote text={cancelBookingText(lease.startDate)} />
        <div className="flex min-w-0 flex-col gap-2">
          <Label htmlFor={`${id}-end`} className={FIELD_LABEL_CLASS}>
            Tanggal berakhir
          </Label>
          <Input
            id={`${id}-end`}
            type="text"
            readOnly
            // Shown, not asked: kept out of the tab order, so the panel does not open with it
            // focused and selected.
            tabIndex={-1}
            value={formatDate(lease.startDate)}
            aria-describedby={`${id}-end-hint`}
            className={cn(INPUT_CLASS, "bg-muted text-muted-foreground")}
          />
          <p id={`${id}-end-hint`} className={FIELD_HINT_CLASS}>
            {CANCEL_BOOKING_HINT}
          </p>
        </div>
      </form>
      <LeasePanelFooter
        formId={formId}
        cancelLabel="Jangan batalkan"
        submitLabel="Batalkan sewa"
        saving={saving}
        wide={false}
        // The user's choice (2026-10-29): cancelling a booking looks destructive.
        destructive
      />
    </>
  );
}

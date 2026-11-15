"use client";

import { useId, useRef, useState, type FormEvent } from "react";
import { Controller, useWatch } from "react-hook-form";

import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import { notify } from "@/components/client/toast/notify";
import { Button } from "@/components/ui/button";
import { useRecordCashDeposit } from "@/lib/api/generated/field/field";
import type { CashTotal } from "@/lib/api/generated/model";

import type { DepositFormOutput } from "./type";
import {
  AMOUNT_HINT,
  AMOUNT_LABEL,
  CASH_CHANGED_MESSAGE,
  DEPOSIT_API_FIELDS,
  DEPOSIT_RECORDED_TOAST,
  depositErrorKind,
  depositFormDefaults,
  depositFormSchema,
  depositRequestBody,
  FOOTER_CAPTION,
  submitLabel,
} from "./utils";

import { useRefreshDeposit } from "./use-refresh-deposit";

type CashDepositFormProps = {
  orgId: string;
  // getMyFieldCash as shown: the amount starts at its total, and the request carries it and its
  // count so the API refuses the deposit if the cash held changed since.
  cash: CashTotal;
  // The deposit is recorded and the screen reloaded.
  onSaved: () => void;
};

// "Jumlah yang Anda serahkan" and the screen's one primary action, "Setor Rp…", pinned above the
// phone's tab bar. A deposit covers every receipt held; the amount is what the karyawan hands
// over, which may differ from the total.
export function CashDepositForm({
  orgId,
  cash,
  onSaved,
}: CashDepositFormProps) {
  const id = useId();
  const captionId = `${id}-caption`;
  const refresh = useRefreshDeposit(orgId);
  const { form, showApiError, alert } = useApiForm({
    schema: depositFormSchema,
    defaultValues: depositFormDefaults(cash),
    apiFields: DEPOSIT_API_FIELDS,
  });
  const amountText = useWatch({ control: form.control, name: "amount" });

  // Made once per form and sent unchanged on a retry, so a double submit or a retry after a lost
  // answer records the deposit once. A new one when the API recorded nothing because the cash
  // changed, since the next request carries another body.
  const [requestId, setRequestId] = useState(() => crypto.randomUUID());

  // The screen behind is reloaded before the form goes away; the toast is raised here, so it
  // shows whatever happens to the form.
  const save = useRecordCashDeposit({
    mutation: {
      onSuccess: () => {
        notify(DEPOSIT_RECORDED_TOAST);
        return refresh().then(() => undefined);
      },
    },
  });
  const saving = save.isPending || save.isSuccess;

  // Set the moment a request goes out, before React renders `saving`: two quick presses both
  // pass the (async) checks, and only the first may send. Cleared when the API refuses.
  const sent = useRef(false);

  async function cashChanged() {
    setRequestId(crypto.randomUUID());
    const fresh = await refresh();
    // With nothing held any more the form is gone; otherwise it starts again at the new total.
    if (fresh !== undefined) form.reset(depositFormDefaults(fresh));
    form.setError("root.server", {
      type: "server",
      message: CASH_CHANGED_MESSAGE,
    });
  }

  function refused(error: unknown) {
    sent.current = false;
    const kind = depositErrorKind(error);
    if (kind === "cash_changed") {
      void cashChanged();
      return;
    }
    if (kind === "nothing_to_deposit" || kind === "request_reused") {
      // The empty state (or the waiting deposit) takes the form's place.
      void refresh();
      return;
    }
    showApiError(error);
  }

  function send(fields: DepositFormOutput) {
    if (saving || sent.current) return;
    sent.current = true;
    save.mutate(
      { orgId, data: depositRequestBody(fields, cash, requestId) },
      { onSuccess: onSaved, onError: refused },
    );
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    void form.handleSubmit(send)(event);
  }

  return (
    <form noValidate onSubmit={submit} className="flex flex-col gap-4">
      {alert !== undefined && <FormAlert message={alert} />}
      <Controller
        control={form.control}
        name="amount"
        render={({ field: amount, fieldState }) => (
          <RupiahField
            inputRef={amount.ref}
            label={AMOUNT_LABEL}
            name="amount"
            placeholder="0"
            hint={AMOUNT_HINT}
            value={amount.value}
            onChange={(text) => {
              amount.onChange(text);
              form.clearErrors("amount");
            }}
            error={fieldState.error?.message}
            disabled={saving}
          />
        )}
      />
      {/* Pinned above the phone's tab bar (64px plus the home indicator; on iOS the floating bar,
          66px above max(24px, the home indicator)); in the column's flow from the iPad width. */}
      <div className="sticky bottom-[calc(env(safe-area-inset-bottom)+64px)] z-10 -mx-4 flex flex-col gap-2 border-t bg-background px-4 pt-3 pb-4 md:static md:mx-0 md:border-t-0 md:px-0 md:pt-1 md:pb-0 ios:max-md:bottom-[calc(max(24px,env(safe-area-inset-bottom))+66px)]">
        <Button
          type="submit"
          disabled={saving}
          aria-describedby={captionId}
          className="h-13 w-full rounded-xl text-[17px] font-bold tabular-nums"
        >
          {saving ? "Menyimpan…" : submitLabel(amountText)}
        </Button>
        <p
          id={captionId}
          className="text-center text-sm text-muted-foreground md:text-base"
        >
          {FOOTER_CAPTION}
        </p>
      </div>
    </form>
  );
}

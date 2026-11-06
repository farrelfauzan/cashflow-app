"use client";

import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";

import { ActivePromiseCard } from "./active-promise-card";
import { ChoiceRadioGroup } from "./choice-radio-group";
import { PaymentPageHeader } from "./payment-page-header";
import { PaymentSummaryCard } from "./payment-summary-card";
import type {
  ActivePromiseView,
  ChoiceOptionView,
  OpenPaymentPageView,
  PayChoice,
} from "./type";
import { ALLOCATION_NOTE, CHOICE_LEGEND, toPayChoice } from "./utils";

type PayChoiceViewProps = {
  view: OpenPaymentPageView;
  choices: readonly ChoiceOptionView[];
  selected: PayChoice;
  onSelect: (choice: PayChoice) => void;
  // "Lanjut bayar Rp2.250.000", or "Lanjut".
  primaryLabel: string;
  onContinue: () => void;
  // The QR status is read once on load: a QR found goes straight to paying, so the choice waits.
  resuming: boolean;
  activePromise: ActivePromiseView | null;
  // "Ubah janji" on the active promise; null when no promise can be made now.
  onEditPromise: (() => void) | null;
};

// Bayar-Pilih (board "Bayar-Pilih"): what is owed, then "Mau bayar berapa?" with one primary that
// follows the choice.
export function PayChoiceView({
  view,
  choices,
  selected,
  onSelect,
  primaryLabel,
  onContinue,
  resuming,
  activePromise,
  onEditPromise,
}: PayChoiceViewProps) {
  return (
    <>
      <PaymentPageHeader header={view.header} />
      {view.linkInvoicePaidNote !== null && (
        <p className="rounded-xl bg-accent px-4 py-3 text-sm font-semibold md:text-base">
          {view.linkInvoicePaidNote}
        </p>
      )}
      <PaymentSummaryCard
        rows={view.rows}
        total={view.total}
        credit={view.credit}
      />
      {activePromise !== null && (
        <ActivePromiseCard text={activePromise.text} onEdit={onEditPromise} />
      )}
      {resuming ? (
        <Skeleton className="h-48 w-full rounded-xl bg-muted" />
      ) : (
        <>
          <ChoiceRadioGroup
            legend={CHOICE_LEGEND}
            legendHidden={false}
            options={choices}
            value={selected}
            onChange={(value) => {
              const choice = toPayChoice(value);
              if (choice !== null) onSelect(choice);
            }}
            error={null}
          />
          <p className="text-sm leading-normal md:text-base">
            {ALLOCATION_NOTE}
          </p>
          <Button
            type="button"
            size="lg"
            className="mt-auto w-full font-semibold"
            onClick={onContinue}
          >
            {primaryLabel}
          </Button>
        </>
      )}
    </>
  );
}

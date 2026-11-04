"use client";

import { TenantChip } from "@/components/client/tenant/tenant-chip";
import { cn } from "@/lib/utils";
import type { InvoiceRowView } from "./type";

import { CorrectButton } from "./correct-button";

type InvoiceRowProps = {
  invoice: InvoiceRowView;
  // Opens "Koreksi nominal" for this invoice; none hides the action.
  onCorrect?: () => void;
};

// One invoice: its month, due date and status, what it bills, what is paid or left, and its
// lines ("Sewa Rp1.050.000"), and "Koreksi" under the status for a member who may correct. The
// amount column keeps its width and stays on the right; a long title ("Tagihan akhir November
// 2026") wraps within its own column instead of pushing the amounts under it.
export function InvoiceRow({ invoice, onCorrect }: InvoiceRowProps) {
  return (
    <li className="flex flex-col gap-1.5 border-t pt-3 first:border-t-0 first:pt-0">
      <div className="flex items-start justify-between gap-3">
        <div className="flex min-w-0 flex-1 flex-col gap-0.5">
          <h3 className="text-base font-bold md:text-[17px]">
            {invoice.title}
          </h3>
          {invoice.place !== null && (
            <span className="text-sm text-muted-foreground md:text-base">
              {invoice.place}
            </span>
          )}
          <span className="text-sm text-muted-foreground md:text-base">
            {invoice.due}
          </span>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-1">
          <span
            className={cn(
              "text-base font-bold md:text-[17px]",
              invoice.struck && "text-muted-foreground line-through",
            )}
          >
            {invoice.amount}
          </span>
          <TenantChip tone={invoice.status.tone}>
            {invoice.status.label}
          </TenantChip>
          {onCorrect !== undefined && (
            <CorrectButton
              label={invoice.correctionTitle}
              onClick={onCorrect}
            />
          )}
        </div>
      </div>
      {invoice.correctedFrom !== null && (
        <span className="text-sm text-muted-foreground md:text-base">
          {invoice.correctedFrom}
        </span>
      )}
      {invoice.details.length > 0 && (
        <p className="text-[15px] font-semibold md:text-base">
          {invoice.details.join(" · ")}
        </p>
      )}
      {invoice.lines.length > 0 && (
        <ul
          aria-label={`Rincian ${invoice.title}`}
          className="flex flex-col gap-0.5"
        >
          {invoice.lines.map((line) => (
            <li
              key={line.id}
              className="flex justify-between gap-3 text-sm text-muted-foreground md:text-base"
            >
              <span className="min-w-0">{line.label}</span>
              <span className="shrink-0">{line.amount}</span>
            </li>
          ))}
        </ul>
      )}
    </li>
  );
}

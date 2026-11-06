"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import type { AppAbility } from "@/lib/capabilities";
import type { CorrectionRequest, CorrectionSelection } from "./type";
import {
  ACCOUNT_FORBIDDEN_TEXT,
  ACCOUNT_LOADING_TEXT,
  ACCOUNT_REGION_LABEL,
  accountTiles,
  canCreateCorrection,
  canReadPromises,
  correctionDialogView,
  correctionRows,
  correctionSavedNotice,
  invoiceRows,
  isAccountForbidden,
  paymentRows,
} from "./utils";

import { AccountTile } from "./account-tile";
import { CorrectionDialog } from "./correction-dialog";
import { CorrectionList } from "./correction-list";
import { InvoiceList } from "./invoice-list";
import { PaymentList } from "./payment-list";
import { PromiseHistory } from "./promise-history";

type TenantAccountSectionProps = {
  orgId: string;
  tenantId: string;
  // For the correction dialog's subtitle.
  tenantName: string;
  // The Jakarta day, YYYY-MM-DD: what is overdue and for how long.
  today: string;
  // The page's short confirmation; "" clears it.
  onNotice: (notice: string) => void;
  // Opens "Koreksi nominal" for an invoice from elsewhere on the page ("Catat meteran"'s locked
  // view); null for none. A new id opens it again.
  correctionRequest: CorrectionRequest | null;
};

// The correction dialog: what it is for, whether it shows, and which opening it is (each
// opening mounts a fresh dialog, so it gets its own requestId).
type CorrectionDialogState = {
  selection: CorrectionSelection;
  open: boolean;
  opening: number;
};

// The tenant's money on Detail penyewa (board "iPad · Detail penyewa"): the totals and the active
// promise as tiles, "Tagihan", "Pembayaran dan alokasi", "Riwayat janji" (for a member with
// promise:read) and "Koreksi nominal". Every figure comes from
// getTenantAccount. The caller renders it only for a member with invoice:read and payment:read.
// A member with correction:create gets "Koreksi" on each invoice and payment, which opens the
// board "iPad · Koreksi nominal" for it.
export function TenantAccountSection({
  orgId,
  tenantId,
  tenantName,
  today,
  onNotice,
  correctionRequest,
}: TenantAccountSectionProps) {
  const account = useGetTenantAccount(orgId, tenantId);
  const ability = useAbility<AppAbility>();
  const canCorrect = canCreateCorrection(ability);
  const showPromises = canReadPromises(ability);
  const [dialog, setDialog] = useState<CorrectionDialogState | null>(null);
  // The last outside request acted on. A new one opens the dialog for its invoice, with a fresh
  // opening (and so its own requestId), as a "Koreksi" button would.
  const [handledRequest, setHandledRequest] = useState<number | null>(null);
  if (
    canCorrect &&
    correctionRequest !== null &&
    correctionRequest.id !== handledRequest
  ) {
    setHandledRequest(correctionRequest.id);
    setDialog((previous) => ({
      selection: { kind: "invoice", id: correctionRequest.invoiceId },
      open: true,
      opening: (previous?.opening ?? 0) + 1,
    }));
  }

  if (account.isError) {
    if (isAccountForbidden(account.error)) {
      return (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {ACCOUNT_FORBIDDEN_TEXT}
        </p>
      );
    }
    return (
      <TenantLoadError
        message={toFormError(account.error, []).message}
        onRetry={() => void account.refetch()}
      />
    );
  }
  if (account.data === undefined) {
    return (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {ACCOUNT_LOADING_TEXT}
      </p>
    );
  }

  const data = account.data;
  const dialogView =
    dialog === null
      ? null
      : correctionDialogView(data, dialog.selection, tenantName);

  function openCorrection(selection: CorrectionSelection) {
    onNotice("");
    setDialog((previous) => ({
      selection,
      open: true,
      opening: (previous?.opening ?? 0) + 1,
    }));
  }
  function closeCorrection() {
    setDialog((previous) =>
      previous === null ? null : { ...previous, open: false },
    );
  }
  const correctInvoice = canCorrect
    ? (id: string) => openCorrection({ kind: "invoice", id })
    : undefined;
  const correctPayment = canCorrect
    ? (id: string) => openCorrection({ kind: "payment", id })
    : undefined;

  return (
    <section
      aria-label={ACCOUNT_REGION_LABEL}
      className="flex flex-col gap-3.5 md:gap-[22px]"
    >
      {/* Phone: "Tunggakan" across the top, the others in pairs; iPad: one row of four. */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
        {accountTiles(data, today).map((tile, index) => (
          <AccountTile
            key={tile.label}
            tile={tile}
            className={index === 0 ? "col-span-2 lg:col-span-1" : undefined}
          />
        ))}
      </div>
      <div className="grid items-start gap-3.5 lg:grid-cols-2 lg:gap-[22px]">
        <InvoiceList
          invoices={invoiceRows(data, today)}
          onCorrect={correctInvoice}
        />
        <PaymentList payments={paymentRows(data)} onCorrect={correctPayment} />
      </div>
      {showPromises && <PromiseHistory orgId={orgId} tenantId={tenantId} />}
      <CorrectionList corrections={correctionRows(data)} />
      {dialog !== null && dialogView !== null && (
        <CorrectionDialog
          key={dialog.opening}
          orgId={orgId}
          tenantId={tenantId}
          view={dialogView}
          open={dialog.open}
          onOpenChange={(open) => {
            if (!open) closeCorrection();
          }}
          onSaved={() => {
            closeCorrection();
            onNotice(correctionSavedNotice(dialogView.title));
          }}
        />
      )}
    </section>
  );
}

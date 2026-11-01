"use client";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useGetTenantAccount } from "@/lib/api/generated/money/money";
import {
  ACCOUNT_FORBIDDEN_TEXT,
  ACCOUNT_LOADING_TEXT,
  ACCOUNT_REGION_LABEL,
  accountTiles,
  correctionRows,
  invoiceRows,
  isAccountForbidden,
  paymentRows,
} from "./utils";

import { AccountTile } from "./account-tile";
import { CorrectionList } from "./correction-list";
import { InvoiceList } from "./invoice-list";
import { PaymentList } from "./payment-list";

type TenantAccountSectionProps = {
  orgId: string;
  tenantId: string;
  // The Jakarta day, YYYY-MM-DD: what is overdue and for how long.
  today: string;
};

// The tenant's money on Detail penyewa (board "iPad · Detail penyewa"), read-only: the totals
// as tiles, "Tagihan", "Pembayaran dan alokasi" and "Koreksi nominal". Every figure comes from
// getTenantAccount. The caller renders it only for a member with invoice:read and payment:read.
export function TenantAccountSection({
  orgId,
  tenantId,
  today,
}: TenantAccountSectionProps) {
  const account = useGetTenantAccount(orgId, tenantId);

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
  return (
    <section
      aria-label={ACCOUNT_REGION_LABEL}
      className="flex flex-col gap-3.5 md:gap-[22px]"
    >
      {/* Phone: "Tunggakan" across the top, the other two side by side; iPad: one row of three. */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-3 lg:gap-4">
        {accountTiles(data, today).map((tile, index) => (
          <AccountTile
            key={tile.label}
            tile={tile}
            className={index === 0 ? "col-span-2 lg:col-span-1" : undefined}
          />
        ))}
      </div>
      <div className="grid items-start gap-3.5 lg:grid-cols-2 lg:gap-[22px]">
        <InvoiceList invoices={invoiceRows(data, today)} />
        <PaymentList payments={paymentRows(data)} />
      </div>
      <CorrectionList corrections={correctionRows(data)} />
    </section>
  );
}

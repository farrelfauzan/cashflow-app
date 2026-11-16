"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { canDepositCash, isForbidden } from "@/components/client/deposit/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import {
  useGetMyFieldCash,
  useListFieldToday,
} from "@/lib/api/generated/field/field";
import type { AppAbility } from "@/lib/capabilities";
import { formatTimestampWeekdayDayMonth } from "@/lib/format/date";

import {
  FIELD_TODAY_PAGE_SIZE,
  FORBIDDEN_TEXT,
  fieldTodayRow,
  moreRowsText,
} from "./utils";

import { FieldCashHeld } from "./field-cash-held";
import { FieldTodayHeader } from "./field-today-header";
import { FieldTodayList } from "./field-today-list";
import { FieldTodaySkeleton } from "./field-today-skeleton";

type FieldTodayViewProps = {
  orgId: string;
};

// Tagihan hari ini (board HP-Tagihan, column 1): today's date, the cash the karyawan holds
// (getMyFieldCash, for a member who may deposit their own cash) and the tenants to collect from
// (listFieldToday, one page). Which tenants are listed is the API's: the caller's own, checked per
// lease.
export function FieldTodayView({ orgId }: FieldTodayViewProps) {
  const ability = useAbility<AppAbility>();
  const [now] = useState(() => new Date());
  const canDeposit = canDepositCash(ability);
  const cash = useGetMyFieldCash(orgId, { query: { enabled: canDeposit } });
  const list = useListFieldToday(orgId, { pageSize: FIELD_TODAY_PAGE_SIZE });

  const header = (
    <FieldTodayHeader date={formatTimestampWeekdayDayMonth(now)} />
  );

  if (isForbidden(list.error)) {
    return (
      <>
        {header}
        <p role="alert" className="text-base">
          {FORBIDDEN_TEXT}
        </p>
      </>
    );
  }
  // The cash card is the karyawan's own; a refusal there hides the card, not the list.
  const failed = [list, ...(canDeposit ? [cash] : [])].find(
    (answer) => answer.error !== null && !isForbidden(answer.error),
  );
  if (failed !== undefined) {
    return (
      <>
        {header}
        <TenantLoadError
          message={toFormError(failed.error, []).message}
          onRetry={() => {
            void list.refetch();
            if (canDeposit) void cash.refetch();
          }}
        />
      </>
    );
  }

  return (
    <>
      {header}
      {canDeposit && cash.data !== undefined && (
        <FieldCashHeld orgId={orgId} cash={cash.data} />
      )}
      {list.data === undefined ? (
        <FieldTodaySkeleton />
      ) : (
        <FieldTodayList
          views={list.data.items.map(fieldTodayRow)}
          moreText={moreRowsText(list.data.total, list.data.items.length)}
        />
      )}
    </>
  );
}

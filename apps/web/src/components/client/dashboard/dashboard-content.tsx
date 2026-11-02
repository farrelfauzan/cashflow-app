"use client";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useGetDashboard } from "@/lib/api/generated/money/money";
import {
  arrearsCount,
  arrearsRows,
  arrearsTotal,
  correctionRows,
  DASHBOARD_LOADING_TEXT,
  DASHBOARD_REGION_LABEL,
  monthMoney,
  moreArrearsText,
} from "./utils";

import { ArrearsList } from "./arrears-list";
import { ArrearsTotalCard } from "./arrears-total-card";
import { MonthMoneyCard } from "./month-money-card";
import { RecentCorrections } from "./recent-corrections";

type DashboardContentProps = {
  orgId: string;
  canOpenTenant: boolean;
};

// The figures of Beranda for this month (Asia/Jakarta, the API's default), all from
// getDashboard. A member limited to some properties gets their properties' figures.
export function DashboardContent({
  orgId,
  canOpenTenant,
}: DashboardContentProps) {
  const dashboard = useGetDashboard(orgId);

  if (dashboard.isError) {
    return (
      <TenantLoadError
        message={toFormError(dashboard.error, []).message}
        onRetry={() => void dashboard.refetch()}
      />
    );
  }
  if (dashboard.data === undefined) {
    return (
      <p className="text-[15px] text-muted-foreground md:text-base">
        {DASHBOARD_LOADING_TEXT}
      </p>
    );
  }

  const data = dashboard.data;
  return (
    <>
      <section
        aria-label={DASHBOARD_REGION_LABEL}
        className="grid gap-3 lg:grid-cols-[minmax(0,1.6fr)_minmax(0,1fr)] lg:gap-4"
      >
        <MonthMoneyCard money={monthMoney(data)} />
        <ArrearsTotalCard total={arrearsTotal(data)} />
      </section>
      <div className="grid items-start gap-3 lg:grid-cols-[minmax(0,1.3fr)_minmax(0,1fr)] lg:gap-4">
        <ArrearsList
          orgId={orgId}
          count={arrearsCount(data)}
          rows={arrearsRows(data)}
          more={moreArrearsText(data)}
          canOpenTenant={canOpenTenant}
        />
        <RecentCorrections corrections={correctionRows(data)} />
      </div>
    </>
  );
}

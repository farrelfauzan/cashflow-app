"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import type { AppAbility } from "@/lib/capabilities";
import { formatLongDate, jakartaHour, jakartaToday } from "@/lib/format/date";
import {
  canOpenTenant,
  canReadDashboard,
  greeting,
  NO_REPORT_TEXT,
} from "./utils";

import { DashboardContent } from "./dashboard-content";
import { DashboardHeader } from "./dashboard-header";

type DashboardViewProps = {
  orgId: string;
};

// Beranda (board "iPad · Beranda Supervisor"): a greeting with the Jakarta day, then, for a
// member who may read reports, the month's money, the tenants in arrears and the newest
// corrections from getDashboard. Approvals, "Uang usaha sekarang", "Telepon admin" and
// "Ingatkan" come with their phases.
export function DashboardView({ orgId }: DashboardViewProps) {
  const ability = useAbility<AppAbility>();
  const [now] = useState(() => new Date());

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <DashboardHeader
        greeting={greeting(jakartaHour(now))}
        date={formatLongDate(jakartaToday(now))}
      />
      {canReadDashboard(ability) ? (
        <DashboardContent
          orgId={orgId}
          canOpenTenant={canOpenTenant(ability)}
        />
      ) : (
        <p className="text-[15px] text-muted-foreground md:text-[17px]">
          {NO_REPORT_TEXT}
        </p>
      )}
    </div>
  );
}

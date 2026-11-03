"use client";

import { useAbility } from "@casl/react";

import { useListMissingMeterReadings } from "@/lib/api/generated/money/money";
import type { AppAbility } from "@/lib/capabilities";
import { meterFlagTitle } from "./utils";

import { MeterFlagBox } from "./meter-flag-box";

type MeterFlagProps = {
  orgId: string;
  // Today in Jakarta, YYYY-MM-DD: names the month the API counts (last month by default).
  today: string;
  onShow: () => void;
};

// "Meteran belum dicatat" on Penagihan: how many leases have no meter amount for last month. Only
// for a member who may read charges, and only while there is any; a failed count shows nothing,
// since the invoices below do not depend on it. The month is left to the API (last month in
// Jakarta).
export function MeterFlag({ orgId, today, onShow }: MeterFlagProps) {
  const ability = useAbility<AppAbility>();
  const canRead = ability.can("read", "charge");
  const missing = useListMissingMeterReadings(
    orgId,
    { pageSize: 1 },
    { query: { enabled: canRead, refetchOnMount: "always" } },
  );
  const total = missing.data?.total ?? 0;
  if (!canRead || total === 0) return null;
  return <MeterFlagBox title={meterFlagTitle(today, total)} onShow={onShow} />;
}

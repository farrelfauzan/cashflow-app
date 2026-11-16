"use client";

import { useAbility } from "@casl/react";
import { useState } from "react";

import { isForbidden } from "@/components/client/deposit/utils";
import { ReceiveCashSheet } from "@/components/client/field-cash/receive-cash-sheet";
import { FieldPromiseSheet } from "@/components/client/field-promise/field-promise-sheet";
import { FieldTodaySkeleton } from "@/components/client/field-today/field-today-skeleton";
import { moreRowsText, placeText } from "@/components/client/field-today/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListFieldVisits } from "@/lib/api/generated/field/field";
import type { FieldVisit } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import type {
  FieldVisitRowView,
  VisitCashSheetItem,
  VisitPromiseSheetItem,
} from "./type";
import {
  FIELD_VISITS_PAGE_SIZE,
  FORBIDDEN_TEXT,
  canRecordVisit,
  fieldVisitSections,
  headerDateLine,
  receiveCashTenant,
} from "./utils";

import { FieldVisitsHeader } from "./field-visits-header";
import { FieldVisitsList } from "./field-visits-list";
import { useTenantOpenings } from "./use-tenant-openings";
import { VisitOutcomeSheet } from "./visit-outcome-sheet";

type FieldVisitsViewProps = {
  orgId: string;
};

// Kunjungan hari ini (board HP-Kunjungan): today's date, the visits still open (listFieldVisits,
// one page, the API's order) and those recorded today. Which tenants are listed is the API's: the
// caller's own, checked per lease.
//
// The sheets live here, not in the rows: a row moves to "Sudah dicatat hari ini" or leaves the
// list while its sheet may still be open or unsure. "Catat hasil kunjungan" opens "Terima tunai"
// and "Catat janji bayar" on top of itself (a second sheet over it on the phone, a second drawer
// over it on the iPad); it stays open beneath, so every choice is still there when the top one
// closes. Both reload the list, and the sheet beneath reads the tenant's row from it: the new
// cash or promise shows inside it at once.
export function FieldVisitsView({ orgId }: FieldVisitsViewProps) {
  const ability = useAbility<AppAbility>();
  const [now] = useState(() => new Date());
  const list = useListFieldVisits(orgId, { pageSize: FIELD_VISITS_PAGE_SIZE });
  const visitSheets = useTenantOpenings<FieldVisit>();
  const cashSheets = useTenantOpenings<VisitCashSheetItem>();
  const [promiseSheet, setPromiseSheet] =
    useState<VisitPromiseSheetItem | null>(null);
  const [promiseOpen, setPromiseOpen] = useState(false);

  // The row as the list holds it now, else as it was when its sheet opened.
  const items = list.data?.items ?? [];
  function currentVisit(fallback: FieldVisit): FieldVisit {
    return (
      items.find((item) => item.tenantId === fallback.tenantId) ?? fallback
    );
  }

  function receiveCash(visit: FieldVisit) {
    cashSheets.start({
      tenantId: visit.tenantId,
      tenant: receiveCashTenant(visit),
      placeLabel: placeText(visit.units),
    });
  }

  function recordPromise(visit: FieldVisit) {
    setPromiseSheet((previous) => ({
      tenantId: visit.tenantId,
      tenant: visit,
      placeLabel: placeText(visit.units),
      key: (previous?.key ?? 0) + 1,
    }));
    setPromiseOpen(true);
  }

  const header = <FieldVisitsHeader dateLine={headerDateLine(now)} />;
  const sheets = (
    <>
      {visitSheets.openings.map((opening) => {
        const visit = currentVisit(opening.item);
        return (
          <VisitOutcomeSheet
            key={opening.requestId}
            orgId={orgId}
            visit={visit}
            requestId={opening.requestId}
            open={
              visitSheets.open &&
              visitSheets.activeTenantId === opening.tenantId
            }
            onOpenChange={visitSheets.setOpen}
            onUnsureChange={(unsure) =>
              visitSheets.markUnsure(opening.tenantId, unsure)
            }
            onReceiveCash={() => receiveCash(visit)}
            onRecordPromise={() => recordPromise(visit)}
          />
        );
      })}
      {cashSheets.openings.map((opening) => (
        <ReceiveCashSheet
          key={opening.requestId}
          orgId={orgId}
          tenant={opening.item.tenant}
          placeLabel={opening.item.placeLabel}
          requestId={opening.requestId}
          open={
            cashSheets.open && cashSheets.activeTenantId === opening.tenantId
          }
          onOpenChange={cashSheets.setOpen}
          onUnsureChange={(unsure) =>
            cashSheets.markUnsure(opening.tenantId, unsure)
          }
        />
      ))}
      {promiseSheet !== null && (
        <FieldPromiseSheet
          key={promiseSheet.key}
          orgId={orgId}
          tenant={promiseSheet.tenant}
          placeLabel={promiseSheet.placeLabel}
          open={promiseOpen}
          onOpenChange={setPromiseOpen}
        />
      )}
    </>
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
  if (list.error !== null && list.data === undefined) {
    return (
      <>
        {header}
        <TenantLoadError
          message={toFormError(list.error, []).message}
          onRetry={() => void list.refetch()}
        />
        {sheets}
      </>
    );
  }

  return (
    <>
      {header}
      {list.data === undefined ? (
        <FieldTodaySkeleton />
      ) : (
        <FieldVisitsList
          sections={fieldVisitSections(list.data.items)}
          moreText={moreRowsText(list.data.total, list.data.items.length)}
          canRecord={canRecordVisit(ability)}
          onRecord={(view: FieldVisitRowView) => visitSheets.start(view.visit)}
        />
      )}
      {sheets}
    </>
  );
}

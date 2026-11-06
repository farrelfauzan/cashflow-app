"use client";

import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListTenantPromises } from "@/lib/api/generated/money/money";
import {
  ACCOUNT_CARD_CLASS,
  isAccountForbidden,
  NO_PROMISES_TEXT,
  PROMISE_HISTORY_LIST_LABEL,
  PROMISE_HISTORY_PAGE_SIZE,
  PROMISE_HISTORY_TITLE,
  PROMISES_FORBIDDEN_TEXT,
  PROMISES_LOADING_TEXT,
  promiseHistoryCountNote,
  promiseHistoryRows,
} from "./utils";

import { PromiseHistoryRow } from "./promise-history-row";

type PromiseHistoryProps = {
  orgId: string;
  tenantId: string;
};

// "Riwayat janji" (board "iPad · Detail penyewa"): the tenant's promises, newest first, from
// listTenantPromises' first page. The caller renders it only for a member with promise:read.
export function PromiseHistory({ orgId, tenantId }: PromiseHistoryProps) {
  const promises = useListTenantPromises(orgId, tenantId, {
    page: 1,
    pageSize: PROMISE_HISTORY_PAGE_SIZE,
  });
  const countNote =
    promises.data === undefined ? null : promiseHistoryCountNote(promises.data);

  return (
    <section className={ACCOUNT_CARD_CLASS}>
      <h2 className="text-xl font-bold md:text-2xl">{PROMISE_HISTORY_TITLE}</h2>
      {promises.isError ? (
        isAccountForbidden(promises.error) ? (
          <p className="text-[15px] text-muted-foreground md:text-base">
            {PROMISES_FORBIDDEN_TEXT}
          </p>
        ) : (
          <TenantLoadError
            message={toFormError(promises.error, []).message}
            onRetry={() => void promises.refetch()}
          />
        )
      ) : promises.data === undefined ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {PROMISES_LOADING_TEXT}
        </p>
      ) : promises.data.items.length === 0 ? (
        <p className="text-[15px] text-muted-foreground md:text-base">
          {NO_PROMISES_TEXT}
        </p>
      ) : (
        <>
          <ul
            aria-label={PROMISE_HISTORY_LIST_LABEL}
            className="flex flex-col gap-3"
          >
            {promiseHistoryRows(promises.data.items).map((promise) => (
              <PromiseHistoryRow key={promise.id} promise={promise} />
            ))}
          </ul>
          {countNote !== null && (
            <p className="text-sm text-muted-foreground md:text-base">
              {countNote}
            </p>
          )}
        </>
      )}
    </section>
  );
}

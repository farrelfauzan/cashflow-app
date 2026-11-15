"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useState } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
} from "@/components/client/list/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListCashDeposits } from "@/lib/api/generated/money/money";
import type { CashDeposit } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import {
  canCountDeposits,
  canReadDeposits,
  CONFIRMED_CAPTION,
  CONFIRMED_EMPTY_TEXT,
  countAction,
  countedRowViews,
  FORBIDDEN_TEXT,
  isForbidden,
  LOADING_TEXT,
  PENDING_EMPTY_TEXT,
  pendingRowView,
  readDepositTab,
  TAB_PARAM,
  tabParam,
} from "./utils";

import { CountDepositSheet } from "./count-deposit-sheet";
import { CountedDepositList } from "./counted-deposit-list";
import { DepositListEmpty } from "./deposit-list-empty";
import { DepositTabs } from "./deposit-tabs";
import { PendingDepositList } from "./pending-deposit-list";

type DepositCountViewProps = {
  orgId: string;
  // The signed-in user (getMe): nobody counts their own deposit.
  userId: string;
};

// The drawer's deposit stays rendered while the sheet slides away; `opening` mounts a fresh form
// (an empty field, a new guard) every time "Hitung" is pressed.
type CountSheetState = {
  open: boolean;
  deposit: CashDeposit | null;
  opening: number;
};

// "Setoran tunai": two tabs over listCashDeposits, "Menunggu dihitung" (with its total as a badge)
// and "Sudah dihitung", one page at a time, the tab, page and size in the URL. "Hitung" opens the
// drawer for a member who may check payments for the whole organization, on deposits that are
// not their own; a member who may only read cash (the Supervisor) sees the same lists without it.
// Anyone else gets one line, as the API would answer them 403. Never by role name.
export function DepositCountView({ orgId, userId }: DepositCountViewProps) {
  const ability = useAbility<AppAbility>();
  const canRead = canReadDeposits(ability);
  const canCount = canCountDeposits(ability);

  const url = useListUrl();
  const tab = readDepositTab(url.searchParams);
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const onPending = tab === "pending";

  // The pending list feeds its tab and the badge; on the other tab it stays at its first page.
  const pending = useListCashDeposits(
    orgId,
    {
      status: "pending",
      page: onPending ? page : 1,
      pageSize: onPending ? pageSize : DEFAULT_PAGE_SIZE,
    },
    { query: { enabled: canRead, placeholderData: keepPreviousData } },
  );
  const confirmed = useListCashDeposits(
    orgId,
    { status: "confirmed", page, pageSize },
    {
      query: {
        enabled: canRead && !onPending,
        placeholderData: keepPreviousData,
      },
    },
  );
  const shown = onPending ? pending : confirmed;
  useLastPageRedirect(url, page, pageSize, shown.data);

  const [sheet, setSheet] = useState<CountSheetState>({
    open: false,
    deposit: null,
    opening: 0,
  });

  function openCount(depositId: string) {
    const deposit = pending.data?.items.find((item) => item.id === depositId);
    if (deposit === undefined) return;
    setSheet((current) => ({
      open: true,
      deposit,
      opening: current.opening + 1,
    }));
  }

  if (!canRead || isForbidden(pending.error) || isForbidden(confirmed.error)) {
    return (
      <p role="alert" className="text-base">
        {FORBIDDEN_TEXT}
      </p>
    );
  }

  let content;
  if (shown.error !== null) {
    content = (
      <TenantLoadError
        message={toFormError(shown.error, []).message}
        onRetry={() => void shown.refetch()}
      />
    );
  } else if (shown.data === undefined) {
    content = (
      <p className="text-base text-muted-foreground" aria-live="polite">
        {LOADING_TEXT}
      </p>
    );
  } else if (shown.data.total === 0) {
    content = (
      <DepositListEmpty
        text={onPending ? PENDING_EMPTY_TEXT : CONFIRMED_EMPTY_TEXT}
      />
    );
  } else {
    content = (
      <div
        aria-busy={shown.isPlaceholderData}
        className="flex flex-col gap-4 aria-busy:opacity-60"
      >
        {onPending ? (
          <PendingDepositList
            rows={shown.data.items.map((deposit) =>
              pendingRowView(deposit, countAction(deposit, userId, canCount)),
            )}
            canCount={canCount}
            onCount={openCount}
          />
        ) : (
          <>
            <CountedDepositList rows={countedRowViews(shown.data.items)} />
            <p className="text-sm text-muted-foreground md:text-base">
              {CONFIRMED_CAPTION}
            </p>
          </>
        )}
        <ListPagination
          page={page}
          pageSize={pageSize}
          total={shown.data.total}
          hrefFor={url.hrefFor}
          onPageSizeChange={(size) => url.change({ [PAGE_SIZE_PARAM]: size })}
        />
      </div>
    );
  }

  return (
    <>
      <DepositTabs
        tab={tab}
        pendingCount={pending.data?.total}
        onTabChange={(next) => url.change({ [TAB_PARAM]: tabParam(next) })}
      >
        {content}
      </DepositTabs>
      {canCount && sheet.deposit !== null && (
        <CountDepositSheet
          key={sheet.opening}
          orgId={orgId}
          deposit={sheet.deposit}
          open={sheet.open}
          onOpenChange={(open) => setSheet((current) => ({ ...current, open }))}
        />
      )}
    </>
  );
}

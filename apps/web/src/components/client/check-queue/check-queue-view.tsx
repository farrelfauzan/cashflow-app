"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useState } from "react";

import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
} from "@/components/client/list/utils";
import { TenantLoadError } from "@/components/client/tenant/tenant-load-error";
import { toFormError } from "@/lib/api/form-error";
import { useListCheckQueue } from "@/lib/api/generated/check-queue/check-queue";
import type { CheckQueueItem } from "@/lib/api/generated/model";
import type { AppAbility } from "@/lib/capabilities";

import {
  canCheckItems,
  canChangeProofPayer,
  canMatchProofs,
  canReadCheckQueue,
  checkRowViews,
  filterOptions,
  FORBIDDEN_TEXT,
  isForbidden,
  KIND_PARAM,
  LOADING_TEXT,
  readCheckKind,
} from "./utils";

import { CheckItemSheet } from "./check-item-sheet";
import { CheckQueueEmpty } from "./check-queue-empty";
import { CheckQueueFilters } from "./check-queue-filters";
import { CheckQueueList } from "./check-queue-list";

type CheckQueueViewProps = {
  orgId: string;
};

// The drawer's item stays rendered while the sheet slides away; `opening` mounts a fresh drawer
// (an empty note, a new guard, the first step) every time a row is opened.
type SheetState = {
  open: boolean;
  item: CheckQueueItem | null;
  opening: number;
};

// "Antrean cek": the waiting items of every kind or one (?kind=), oldest first, one page at a
// time, with the kind, page and size in the URL. A row opens its kind's drawer; a member who may
// check payments for the whole organization answers it there, one who may only read cash (the
// Supervisor) opens the same drawers read-only. Anyone else gets one line, as the API would
// answer them 403. Never by role name.
export function CheckQueueView({ orgId }: CheckQueueViewProps) {
  const ability = useAbility<AppAbility>();
  const canRead = canReadCheckQueue(ability);
  const canAct = canCheckItems(ability);
  const canMatch = canMatchProofs(ability);
  const canChangePayer = canChangeProofPayer(ability);

  const url = useListUrl();
  const kind = readCheckKind(url.searchParams);
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);

  const list = useListCheckQueue(
    orgId,
    { kind, page, pageSize },
    { query: { enabled: canRead, placeholderData: keepPreviousData } },
  );
  useLastPageRedirect(url, page, pageSize, list.data);

  const [sheet, setSheet] = useState<SheetState>({
    open: false,
    item: null,
    opening: 0,
  });

  function openItem(itemId: string) {
    const item = list.data?.items.find((candidate) => candidate.id === itemId);
    if (item === undefined) return;
    setSheet((current) => ({ open: true, item, opening: current.opening + 1 }));
  }

  if (!canRead || isForbidden(list.error)) {
    return (
      <p role="alert" className="text-base">
        {FORBIDDEN_TEXT}
      </p>
    );
  }

  let content;
  if (list.error !== null) {
    content = (
      <TenantLoadError
        message={toFormError(list.error, []).message}
        onRetry={() => void list.refetch()}
      />
    );
  } else if (list.data === undefined) {
    content = (
      <p className="text-base text-muted-foreground" aria-live="polite">
        {LOADING_TEXT}
      </p>
    );
  } else if (list.data.total === 0) {
    content = <CheckQueueEmpty />;
  } else {
    content = (
      <div
        aria-busy={list.isPlaceholderData}
        className="flex flex-col gap-4 aria-busy:opacity-60"
      >
        <CheckQueueList
          rows={checkRowViews(list.data.items)}
          onOpen={openItem}
        />
        <ListPagination
          page={page}
          pageSize={pageSize}
          total={list.data.total}
          hrefFor={url.hrefFor}
          onPageSizeChange={(size) => url.change({ [PAGE_SIZE_PARAM]: size })}
        />
      </div>
    );
  }

  return (
    <>
      <CheckQueueFilters
        options={filterOptions(list.data?.counts, list.data?.waiting)}
        selected={kind}
        onSelect={(next) => url.change({ [KIND_PARAM]: next })}
      />
      {content}
      {sheet.item !== null && (
        <CheckItemSheet
          key={sheet.opening}
          orgId={orgId}
          item={sheet.item}
          canAct={canAct}
          canMatch={canMatch}
          canChangePayer={canChangePayer}
          open={sheet.open}
          onOpenChange={(open) => setSheet((current) => ({ ...current, open }))}
        />
      )}
    </>
  );
}

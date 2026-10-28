"use client";

import { useEffect } from "react";

import type { ListUrl } from "./type";
import { PAGE_PARAM, pageAfterOverflow } from "./utils";

// When the API answers a page past the end (an old link, or the last row of the last page was
// just removed), it sends no rows and the real total: go to the last page that has rows instead
// of showing an empty table. Replaces the history entry, so back skips the empty page.
export function useLastPageRedirect(
  url: ListUrl,
  page: number,
  pageSize: number,
  answer: { total: number; items: readonly unknown[] } | undefined,
): void {
  const target =
    answer === undefined
      ? null
      : pageAfterOverflow(page, pageSize, answer.total, answer.items.length);
  const { change } = url;
  useEffect(() => {
    if (target !== null) {
      change({ [PAGE_PARAM]: target }, { replace: true });
    }
  }, [change, target]);
}

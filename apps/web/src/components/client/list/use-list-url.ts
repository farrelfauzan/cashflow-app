"use client";

import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback } from "react";

import type { ListUrl } from "./type";
import { listHref, PAGE_PARAM } from "./utils";

// A paginated list's state in the URL: the current params, the link to a page (for the page
// bar's links), and `change` for a search, a filter, the sort or the page size. A change pushes
// a history entry, so back undoes it; `replace` is for the search, which changes as the member
// types. The page does not scroll, so the filters stay where the member is looking.
export function useListUrl(): ListUrl {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const router = useRouter();

  const hrefFor = useCallback(
    (page: number) => listHref(pathname, searchParams, { [PAGE_PARAM]: page }),
    [pathname, searchParams],
  );

  const change = useCallback<ListUrl["change"]>(
    (changes, options) => {
      const href = listHref(pathname, searchParams, changes);
      if (options?.replace === true) {
        router.replace(href, { scroll: false });
      } else {
        router.push(href, { scroll: false });
      }
    },
    [pathname, router, searchParams],
  );

  return { searchParams, hrefFor, change };
}

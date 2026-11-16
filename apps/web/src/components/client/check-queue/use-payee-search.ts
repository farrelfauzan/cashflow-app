"use client";

import { keepPreviousData } from "@tanstack/react-query";

import { useDebouncedValue } from "@/components/client/form/use-debounced-value";
import { toFormError } from "@/lib/api/form-error";
import { useListTenants } from "@/lib/api/generated/tenants/tenants";

import type { ProofPayeeSearchView } from "./type";
import {
  PAYEE_SEARCH_PAGE_SIZE,
  payeeSearchQuery,
  payeeSearchView,
} from "./utils";

type PayeeSearchOptions = {
  orgId: string;
  // The proof's sender, listed first on their own and left out of the results.
  senderId: string;
  // The search as typed.
  text: string;
};

type PayeeSearch = {
  // The tenants found for the last search; null before anything is typed or while the first
  // answer loads.
  view: ProofPayeeSearchView | null;
  // Typing has not paused yet, or a search is on its way.
  loading: boolean;
  // The API's refusal of the search, in Bahasa Indonesia.
  error: string | null;
};

// "Cari penyewa": listTenants with the typed text as q (name, unit, or WhatsApp number, matched on
// the server), once typing has paused. Nothing is searched while the field is empty.
export function usePayeeSearch({
  orgId,
  senderId,
  text,
}: PayeeSearchOptions): PayeeSearch {
  const query = payeeSearchQuery(text);
  const debounced = useDebouncedValue(query);
  const settled = debounced === query;
  const list = useListTenants(
    orgId,
    { q: debounced ?? "", pageSize: PAYEE_SEARCH_PAGE_SIZE },
    {
      query: {
        enabled: debounced !== null,
        placeholderData: keepPreviousData,
      },
    },
  );

  if (query === null) return { view: null, loading: false, error: null };
  const error =
    settled && list.isError ? toFormError(list.error, []).message : null;
  const data = list.isError ? undefined : list.data;
  return {
    view: data === undefined ? null : payeeSearchView(data, senderId),
    loading: !settled || list.isFetching,
    error,
  };
}

"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { PromiseForm } from "@/components/client/billing/promise-form";
import { PromiseSheetHeader } from "@/components/client/billing/promise-sheet-header";
import type { PromiseDialogView } from "@/components/client/billing/type";
import {
  isPromiseStaleError,
  PROMISE_SHEET_CLASSES,
  promiseSavedToast,
} from "@/components/client/billing/utils";
import { sheetSubtitle } from "@/components/client/field-cash/utils";
import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { notify } from "@/components/client/toast/notify";
import { toFormError } from "@/lib/api/form-error";
import {
  getGetFieldPromiseOfferQueryKey,
  getListFieldTodayQueryKey,
  useGetFieldPromiseOffer,
  useMakeFieldPromise,
} from "@/lib/api/generated/field/field";

import type { FieldPromiseTenant } from "./type";
import {
  fieldPromiseDialogView,
  NO_FIELD_PROMISE_TEXT,
  RECORD_FIELD_PROMISE_TEXT,
} from "./utils";

import { FieldPromiseSkeleton } from "./field-promise-skeleton";
import { FieldPromiseUnavailable } from "./field-promise-unavailable";

type FieldPromiseSheetProps = {
  orgId: string;
  tenant: FieldPromiseTenant;
  // The row's place line ("Kamar 15 · Kos Melati"), repeated in the subtitle.
  placeLabel: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

const CLASSES = PROMISE_SHEET_CLASSES.drawer;

// "Catat janji bayar" (board HP-Tagihan, column 4): a bottom sheet on the phone, the right-hand
// drawer from md up, like "Terima tunai". The caller mounts it afresh for each opening; it reads
// getFieldPromiseOffer then (never the cached answer of an earlier opening: a payment in between
// moves the dates) and shows a skeleton until it arrives. The form is made once from that answer
// and kept while the sheet is open. Saving sends makeFieldPromise (recorded as the karyawan's),
// raises the toast and reloads Tagihan hari ini and the offer.
export function FieldPromiseSheet({
  orgId,
  tenant,
  placeLabel,
  open,
  onOpenChange,
}: FieldPromiseSheetProps) {
  const queryClient = useQueryClient();
  const subtitle = sheetSubtitle(tenant.tenantName, placeLabel);
  const offer = useGetFieldPromiseOffer(orgId, tenant.tenantId, {
    query: { refetchOnMount: "always", refetchOnWindowFocus: false },
  });
  // Frozen when this opening's answer arrives, so a refetch never resets what is typed.
  const [view, setView] = useState<PromiseDialogView | null | undefined>(
    undefined,
  );
  if (view === undefined && offer.isSuccess && offer.isFetchedAfterMount) {
    setView(fieldPromiseDialogView(offer.data, tenant, subtitle));
  }

  const reload = () =>
    Promise.all(
      [
        getListFieldTodayQueryKey(orgId),
        getGetFieldPromiseOfferQueryKey(orgId, tenant.tenantId),
      ].map((queryKey) => queryClient.invalidateQueries({ queryKey })),
    );
  const make = useMakeFieldPromise({
    mutation: {
      // Raised here, not in mutate()'s callback, so it shows even if the sheet is gone by the
      // time the API answers. Over an active promise, saving replaced it.
      onSuccess: () => {
        notify(
          promiseSavedToast(
            view !== undefined && view !== null && view.replacesLine !== null,
            tenant.tenantName,
          ),
        );
        return reload();
      },
    },
  });
  const saving = make.isPending;

  // Why no form shows: the API refused the offer (403, 404: its own message), or no promise can
  // be made now. Null while it loads (the skeleton) or once the form shows.
  const unavailable =
    view === null
      ? NO_FIELD_PROMISE_TEXT
      : view === undefined && offer.isError
        ? toFormError(offer.error, []).message
        : null;

  return (
    <FormSheet open={open} onOpenChange={onOpenChange} busy={saving}>
      <FormSheetContent showCloseButton={false}>
        {view === undefined || view === null ? (
          <>
            <PromiseSheetHeader
              title={RECORD_FIELD_PROMISE_TEXT}
              subtitle={subtitle}
              replacesLine={null}
              className={CLASSES.header}
              closeDisabled={false}
            />
            {unavailable === null ? (
              <FieldPromiseSkeleton className={CLASSES.body} />
            ) : (
              <FieldPromiseUnavailable
                message={unavailable}
                className={CLASSES.body}
              />
            )}
          </>
        ) : (
          <PromiseForm
            view={view}
            layout="drawer"
            saving={saving}
            onSave={(data, onError) => {
              make.mutate(
                { orgId, tenantId: tenant.tenantId, data },
                {
                  onSuccess: () => onOpenChange(false),
                  onError: (error) => {
                    // The row behind the sheet is out of date; the sheet keeps the message.
                    if (isPromiseStaleError(error)) void reload();
                    onError(error);
                  },
                },
              );
            }}
          />
        )}
      </FormSheetContent>
    </FormSheet>
  );
}

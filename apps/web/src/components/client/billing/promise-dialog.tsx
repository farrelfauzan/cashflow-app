"use client";

import { useQueryClient } from "@tanstack/react-query";

import { FormSheet } from "@/components/client/form-sheet/form-sheet";
import { FormSheetContent } from "@/components/client/form-sheet/form-sheet-content";
import { notify } from "@/components/client/toast/notify";
import {
  getGetTenantAccountQueryKey,
  getListTenantPromisesQueryKey,
  useMakeTenantPromise,
} from "@/lib/api/generated/money/money";
import type { PromiseDialogView } from "./type";
import {
  isInvoicesQuery,
  isPromiseStaleError,
  promiseSavedToast,
} from "./utils";

import { PromiseForm } from "./promise-form";

type PromiseDialogProps = {
  orgId: string;
  tenantId: string;
  // Made from the account when the dialog opened.
  view: PromiseDialogView;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  // After the promise is saved and the queries that show it are reloaded.
  onSaved: () => void;
};

// "Catat janji" (no board: a design gap, built like "Koreksi nominal" with the fields of the
// tenant's Janji screen): when the tenant pays what is left and, optionally, how much, with the
// member's note, sent with makeTenantPromise. The fields are PromiseForm's, shared with the field
// app's "Catat janji bayar". It stays open while the request runs. A centred dialog from md up;
// on the phone a form sheet (board HP-Lembar) with Batal and the save button pinned at the
// bottom, which asks before a changed promise is closed unsaved.
export function PromiseDialog({
  orgId,
  tenantId,
  view,
  open,
  onOpenChange,
  onSaved,
}: PromiseDialogProps) {
  const queryClient = useQueryClient();
  const reloadAccount = () =>
    queryClient.invalidateQueries({
      queryKey: getGetTenantAccountQueryKey(orgId, tenantId),
    });
  // The account's tile, "Riwayat janji" and Penagihan's rows show the promise. Nothing cached is
  // patched; the API's answer is the one shown.
  const make = useMakeTenantPromise({
    mutation: {
      // The toast is raised here, not in mutate()'s callback, so it shows even if the dialog is
      // gone by the time the API answers. Over an active promise, saving replaced it.
      onSuccess: () => {
        notify(promiseSavedToast(view.replacesLine !== null, view.subtitle));
        return Promise.all([
          reloadAccount(),
          queryClient.invalidateQueries({
            queryKey: getListTenantPromisesQueryKey(orgId, tenantId),
          }),
          queryClient.invalidateQueries({
            predicate: (query) => isInvoicesQuery(query.queryKey, orgId),
          }),
        ]);
      },
    },
  });
  const saving = make.isPending;

  return (
    <FormSheet open={open} onOpenChange={onOpenChange} busy={saving}>
      <FormSheetContent wide="dialog" showCloseButton={false}>
        <PromiseForm
          view={view}
          layout="dialog"
          saving={saving}
          onSave={(body, onError) => {
            make.mutate(
              { orgId, tenantId, data: body },
              {
                onSuccess: onSaved,
                onError: (error) => {
                  // What the page shows behind the dialog is out of date; the dialog keeps the
                  // message.
                  if (isPromiseStaleError(error)) void reloadAccount();
                  onError(error);
                },
              },
            );
          }}
        />
      </FormSheetContent>
    </FormSheet>
  );
}

"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  TEXTAREA_CLASS,
} from "@/components/client/form/utils";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import type { Tenant } from "@/lib/api/generated/model";
import {
  getGetTenantQueryKey,
  getListTenantsQueryKey,
  useCreateTenant,
  useUpdateTenant,
} from "@/lib/api/generated/tenants/tenants";
import {
  TENANT_FORM_API_FIELDS,
  TENANT_NAME_MAX_LENGTH,
  TENANT_NOTE_MAX_LENGTH,
  tenantFormSchema,
  tenantFormValues,
} from "./utils";

type TenantFormProps = {
  orgId: string;
  // The tenant being changed; none when adding.
  tenant: Tenant | undefined;
  onSaved: (tenant: Tenant) => void;
};

// "Tambah penyewa" and "Ubah penyewa" (board Penyewa-Tambah.dc.html): name, WhatsApp number and
// a note. The save button sits outside the form, in the panel's footer, and submits it through
// its `form` attribute.
export function TenantForm({ orgId, tenant, onSaved }: TenantFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: tenantFormSchema,
    defaultValues: tenantFormValues(tenant),
    apiFields: TENANT_FORM_API_FIELDS,
  });
  const { errors } = form.formState;
  const nameError = errors.name?.message;
  const phoneError = errors.phone?.message;
  const noteError = errors.note?.message;

  const invalidateList = () =>
    queryClient.invalidateQueries({ queryKey: getListTenantsQueryKey(orgId) });
  const create = useCreateTenant({ mutation: { onSuccess: invalidateList } });
  const update = useUpdateTenant({
    mutation: {
      onSuccess: (saved) =>
        Promise.all([
          invalidateList(),
          queryClient.invalidateQueries({
            queryKey: getGetTenantQueryKey(orgId, saved.id),
          }),
        ]),
    },
  });
  // After adding, the panel moves on to the next step; the form stays locked until then.
  const saving = create.isPending || create.isSuccess || update.isPending;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    const options = { onSuccess: onSaved, onError: showApiError };
    if (tenant === undefined) {
      create.mutate({ orgId, data: input }, options);
    } else {
      update.mutate({ orgId, tenantId: tenant.id, data: input }, options);
    }
  });

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={submit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== undefined && <FormAlert message={alert} />}

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-name`} className={FIELD_LABEL_CLASS}>
            Nama penyewa
          </Label>
          <Input
            id={`${id}-name`}
            type="text"
            autoComplete="off"
            maxLength={TENANT_NAME_MAX_LENGTH}
            placeholder="Contoh: Bu Dewi Lestari"
            {...field("name")}
            disabled={saving}
            aria-invalid={nameError !== undefined}
            aria-describedby={
              nameError !== undefined ? `${id}-name-error` : undefined
            }
            className={INPUT_CLASS}
          />
          {nameError !== undefined && (
            <FieldError id={`${id}-name-error`} message={nameError} />
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-phone`} className={FIELD_LABEL_CLASS}>
            Nomor WhatsApp
          </Label>
          <Input
            id={`${id}-phone`}
            type="tel"
            inputMode="tel"
            autoComplete="off"
            placeholder="0812-3456-7890"
            {...field("phone")}
            disabled={saving}
            aria-invalid={phoneError !== undefined}
            aria-describedby={
              phoneError !== undefined
                ? `${id}-phone-error ${id}-phone-hint`
                : `${id}-phone-hint`
            }
            className={INPUT_CLASS}
          />
          {phoneError !== undefined && (
            <FieldError id={`${id}-phone-error`} message={phoneError} />
          )}
          <p id={`${id}-phone-hint`} className={FIELD_HINT_CLASS}>
            Tulis seperti biasa, misalnya 0812-3456-7890 atau +62 812 3456 7890.
            Pesan tagihan dikirim ke nomor ini.
          </p>
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-note`} className={FIELD_LABEL_CLASS}>
            Catatan (boleh dikosongkan)
          </Label>
          <Textarea
            id={`${id}-note`}
            autoComplete="off"
            maxLength={TENANT_NOTE_MAX_LENGTH}
            placeholder="Contoh: kerja di toko, bayar lewat anaknya"
            {...field("note")}
            disabled={saving}
            aria-invalid={noteError !== undefined}
            aria-describedby={
              noteError !== undefined ? `${id}-note-error` : undefined
            }
            className={TEXTAREA_CLASS}
          />
          {noteError !== undefined && (
            <FieldError id={`${id}-note-error`} message={noteError} />
          )}
        </div>
      </form>
      <div className="flex gap-2.5 border-t px-6 py-4">
        <SheetClose asChild>
          <Button
            type="button"
            variant="outline"
            className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
          >
            Batal
          </Button>
        </SheetClose>
        <Button
          type="submit"
          form={formId}
          disabled={saving}
          className="h-13 flex-2 rounded-xl px-3 text-[17px] font-semibold whitespace-normal"
        >
          {saving ? "Menyimpan…" : "Simpan penyewa"}
        </Button>
      </div>
    </>
  );
}

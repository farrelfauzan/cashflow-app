"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type FormEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
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
import { toFormError } from "@/lib/api/form-error";
import type { Tenant } from "@/lib/api/generated/model";
import {
  getGetTenantQueryKey,
  getListTenantsQueryKey,
  useCreateTenant,
  useUpdateTenant,
} from "@/lib/api/generated/tenants/tenants";
import type { TenantFormErrors } from "./type";
import {
  TENANT_FORM_FIELDS,
  TENANT_NAME_MAX_LENGTH,
  TENANT_NOTE_MAX_LENGTH,
  checkTenantForm,
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
  const nameRef = useRef<HTMLInputElement>(null);
  const phoneRef = useRef<HTMLInputElement>(null);
  const noteRef = useRef<HTMLTextAreaElement>(null);
  const queryClient = useQueryClient();
  const [values, setValues] = useState(() => tenantFormValues(tenant));
  const [errors, setErrors] = useState<TenantFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

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

  function change(field: keyof TenantFormErrors, value: string) {
    setValues((current) => ({ ...current, [field]: value }));
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  // The field to focus after a refused save. The fields are disabled while saving, so the focus
  // waits until they are enabled again; a new object each time, so the same field can repeat.
  const [focusTarget, setFocusTarget] = useState<{
    field: keyof TenantFormErrors;
  } | null>(null);
  useEffect(() => {
    if (focusTarget === null || saving) return;
    const ref = { name: nameRef, phone: phoneRef, note: noteRef }[
      focusTarget.field
    ];
    ref.current?.focus();
  }, [focusTarget, saving]);

  function handleError(error: unknown) {
    const failed = toFormError(error, TENANT_FORM_FIELDS);
    if (
      failed.field === "name" ||
      failed.field === "phone" ||
      failed.field === "note"
    ) {
      setErrors({ [failed.field]: failed.message });
      setFocusTarget({ field: failed.field });
    } else {
      setAlert(failed.message);
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkTenantForm(values);
    if (!checked.ok) {
      setErrors(checked.errors);
      setFocusTarget({
        field: checked.errors.name !== undefined ? "name" : "phone",
      });
      return;
    }
    setErrors({});

    const options = { onSuccess: onSaved, onError: handleError };
    if (tenant === undefined) {
      create.mutate({ orgId, data: checked.input }, options);
    } else {
      update.mutate(
        { orgId, tenantId: tenant.id, data: checked.input },
        options,
      );
    }
  }

  return (
    <>
      <form
        id={formId}
        noValidate
        onSubmit={handleSubmit}
        className="flex min-h-0 flex-1 flex-col gap-[18px] overflow-y-auto px-6 py-5"
      >
        {alert !== null && <FormAlert message={alert} />}

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-name`} className={FIELD_LABEL_CLASS}>
            Nama penyewa
          </Label>
          <Input
            ref={nameRef}
            id={`${id}-name`}
            name="name"
            type="text"
            autoComplete="off"
            maxLength={TENANT_NAME_MAX_LENGTH}
            placeholder="Contoh: Bu Dewi Lestari"
            value={values.name}
            onChange={(event) => change("name", event.target.value)}
            disabled={saving}
            aria-invalid={errors.name !== undefined}
            aria-describedby={
              errors.name !== undefined ? `${id}-name-error` : undefined
            }
            className={INPUT_CLASS}
          />
          {errors.name !== undefined && (
            <FieldError id={`${id}-name-error`} message={errors.name} />
          )}
        </div>

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-phone`} className={FIELD_LABEL_CLASS}>
            Nomor WhatsApp
          </Label>
          <Input
            ref={phoneRef}
            id={`${id}-phone`}
            name="phone"
            type="tel"
            inputMode="tel"
            autoComplete="off"
            placeholder="0812-3456-7890"
            value={values.phone}
            onChange={(event) => change("phone", event.target.value)}
            disabled={saving}
            aria-invalid={errors.phone !== undefined}
            aria-describedby={
              errors.phone !== undefined
                ? `${id}-phone-error ${id}-phone-hint`
                : `${id}-phone-hint`
            }
            className={INPUT_CLASS}
          />
          {errors.phone !== undefined && (
            <FieldError id={`${id}-phone-error`} message={errors.phone} />
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
            ref={noteRef}
            id={`${id}-note`}
            name="note"
            autoComplete="off"
            maxLength={TENANT_NOTE_MAX_LENGTH}
            placeholder="Contoh: kerja di toko, bayar lewat anaknya"
            value={values.note}
            onChange={(event) => change("note", event.target.value)}
            disabled={saving}
            aria-invalid={errors.note !== undefined}
            aria-describedby={
              errors.note !== undefined ? `${id}-note-error` : undefined
            }
            className={TEXTAREA_CLASS}
          />
          {errors.note !== undefined && (
            <FieldError id={`${id}-note-error`} message={errors.note} />
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

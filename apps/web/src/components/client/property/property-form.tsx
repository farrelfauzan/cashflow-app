"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import type { Property } from "@/lib/api/generated/model";
import {
  getGetPropertyQueryKey,
  getListPropertiesQueryKey,
  useCreateProperty,
  useUpdateProperty,
} from "@/lib/api/generated/properties/properties";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  PROPERTY_ADDRESS_MAX_LENGTH,
  PROPERTY_NAME_MAX_LENGTH,
  TEXTAREA_CLASS,
  propertyFormSchema,
} from "./utils";

import { PropertyKindField } from "./property-kind-field";

type PropertyFormProps = {
  orgId: string;
  // The property being changed; none when adding.
  property: Property | undefined;
  onSaved: (property: Property) => void;
};

// The fields the API's errors can name; any other field goes to the alert.
const FORM_FIELDS = ["name", "kind", "address"] as const;

// "Tambah properti" and "Ubah properti": name, kind and address. The save button sits outside
// the form, in the panel's footer, and submits it through its `form` attribute.
export function PropertyForm({ orgId, property, onSaved }: PropertyFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: propertyFormSchema,
    defaultValues: {
      name: property?.name ?? "",
      kind: property?.kind,
      address: property?.address ?? "",
    },
    apiFields: FORM_FIELDS,
  });
  const { errors } = form.formState;
  const nameError = errors.name?.message;
  const addressError = errors.address?.message;

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: getListPropertiesQueryKey(orgId),
    });
  // A changed property also refreshes its own page (getProperty), which the list key misses.
  const invalidateOne = (saved: Property) =>
    queryClient.invalidateQueries({
      queryKey: getGetPropertyQueryKey(orgId, saved.id),
    });
  const create = useCreateProperty({ mutation: { onSuccess: invalidate } });
  const update = useUpdateProperty({
    mutation: {
      onSuccess: (saved) => Promise.all([invalidate(), invalidateOne(saved)]),
    },
  });
  const saving = create.isPending || update.isPending;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    const options = { onSuccess: onSaved, onError: showApiError };
    if (property === undefined) {
      create.mutate({ orgId, data: input }, options);
    } else {
      update.mutate({ orgId, propertyId: property.id, data: input }, options);
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
            Nama properti
          </Label>
          <Input
            id={`${id}-name`}
            type="text"
            autoComplete="off"
            maxLength={PROPERTY_NAME_MAX_LENGTH}
            placeholder="Contoh: Kos Melati"
            {...field("name")}
            disabled={saving}
            aria-invalid={nameError !== undefined}
            aria-describedby={
              nameError !== undefined
                ? `${id}-name-error ${id}-name-hint`
                : `${id}-name-hint`
            }
            className={INPUT_CLASS}
          />
          {nameError !== undefined && (
            <FieldError id={`${id}-name-error`} message={nameError} />
          )}
          <p id={`${id}-name-hint`} className={FIELD_HINT_CLASS}>
            Satu nama hanya untuk satu properti.
            <span className="hidden md:inline">
              {" "}
              Huruf besar dan kecil dianggap sama.
            </span>
          </p>
        </div>

        <Controller
          control={form.control}
          name="kind"
          render={({ field: kind }) => (
            <PropertyKindField
              value={kind.value ?? null}
              onChange={(next) => {
                kind.onChange(next);
                form.clearErrors("kind");
              }}
              error={errors.kind?.message}
              disabled={saving}
            />
          )}
        />

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-address`} className={FIELD_LABEL_CLASS}>
            Alamat (boleh dikosongkan)
          </Label>
          <Textarea
            id={`${id}-address`}
            autoComplete="off"
            maxLength={PROPERTY_ADDRESS_MAX_LENGTH}
            placeholder="Contoh: Jl. Melati No. 12, Sleman"
            {...field("address")}
            disabled={saving}
            aria-invalid={addressError !== undefined}
            aria-describedby={
              addressError !== undefined
                ? `${id}-address-error ${id}-address-hint`
                : `${id}-address-hint`
            }
            className={TEXTAREA_CLASS}
          />
          {addressError !== undefined && (
            <FieldError id={`${id}-address-error`} message={addressError} />
          )}
          <p id={`${id}-address-hint`} className={FIELD_HINT_CLASS}>
            Dipakai di pesan WhatsApp dan laporan.
            <span className="hidden md:inline">
              {" "}
              Paling banyak {PROPERTY_ADDRESS_MAX_LENGTH} huruf.
            </span>
          </p>
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
          {saving ? "Menyimpan…" : "Simpan properti"}
        </Button>
      </div>
    </>
  );
}

"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type FormEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { toFormError } from "@/lib/api/form-error";
import type { Property, PropertyKind } from "@/lib/api/generated/model";
import {
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
  checkPropertyForm,
} from "./utils";
import type { PropertyFormErrors } from "./type";

import { PropertyKindField } from "./property-kind-field";

type PropertyFormProps = {
  orgId: string;
  // The property being changed; none when adding.
  property: Property | undefined;
  onSaved: (property: Property) => void;
};

// The fields the API's errors can name; any other field goes to the alert.
const FORM_FIELDS = ["name", "kind", "address"];

// "Tambah properti" and "Ubah properti": name, kind and address. The save button sits outside
// the form, in the panel's footer, and submits it through its `form` attribute.
export function PropertyForm({ orgId, property, onSaved }: PropertyFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const nameRef = useRef<HTMLInputElement>(null);
  const addressRef = useRef<HTMLTextAreaElement>(null);
  const queryClient = useQueryClient();
  const [name, setName] = useState(property?.name ?? "");
  const [kind, setKind] = useState<PropertyKind | null>(property?.kind ?? null);
  const [address, setAddress] = useState(property?.address ?? "");
  const [errors, setErrors] = useState<PropertyFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

  const invalidate = () =>
    queryClient.invalidateQueries({
      queryKey: getListPropertiesQueryKey(orgId),
    });
  const create = useCreateProperty({ mutation: { onSuccess: invalidate } });
  const update = useUpdateProperty({ mutation: { onSuccess: invalidate } });
  const saving = create.isPending || update.isPending;

  function clearError(field: keyof PropertyFormErrors) {
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, FORM_FIELDS);
    if (failed.field === null) {
      setAlert(failed.message);
      return;
    }
    setErrors({ [failed.field]: failed.message });
    if (failed.field === "name") nameRef.current?.focus();
    if (failed.field === "address") addressRef.current?.focus();
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkPropertyForm({ name, kind, address });
    if (!checked.ok) {
      setErrors(checked.errors);
      if (checked.errors.name !== undefined) nameRef.current?.focus();
      return;
    }
    setErrors({});

    const options = { onSuccess: onSaved, onError: handleError };
    if (property === undefined) {
      create.mutate({ orgId, data: checked.input }, options);
    } else {
      update.mutate(
        { orgId, propertyId: property.id, data: checked.input },
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
            Nama properti
          </Label>
          <Input
            ref={nameRef}
            id={`${id}-name`}
            name="name"
            type="text"
            autoComplete="off"
            maxLength={PROPERTY_NAME_MAX_LENGTH}
            placeholder="Contoh: Kos Melati"
            value={name}
            onChange={(event) => {
              setName(event.target.value);
              clearError("name");
            }}
            disabled={saving}
            aria-invalid={errors.name !== undefined}
            aria-describedby={
              errors.name !== undefined
                ? `${id}-name-error ${id}-name-hint`
                : `${id}-name-hint`
            }
            className={INPUT_CLASS}
          />
          {errors.name !== undefined && (
            <FieldError id={`${id}-name-error`} message={errors.name} />
          )}
          <p id={`${id}-name-hint`} className={FIELD_HINT_CLASS}>
            Satu nama hanya untuk satu properti.
            <span className="hidden md:inline">
              {" "}
              Huruf besar dan kecil dianggap sama.
            </span>
          </p>
        </div>

        <PropertyKindField
          value={kind}
          onChange={(next) => {
            setKind(next);
            clearError("kind");
          }}
          error={errors.kind}
          disabled={saving}
        />

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-address`} className={FIELD_LABEL_CLASS}>
            Alamat (boleh dikosongkan)
          </Label>
          <Textarea
            ref={addressRef}
            id={`${id}-address`}
            name="address"
            autoComplete="off"
            maxLength={PROPERTY_ADDRESS_MAX_LENGTH}
            placeholder="Contoh: Jl. Melati No. 12, Sleman"
            value={address}
            onChange={(event) => {
              setAddress(event.target.value);
              clearError("address");
            }}
            disabled={saving}
            aria-invalid={errors.address !== undefined}
            aria-describedby={
              errors.address !== undefined
                ? `${id}-address-error ${id}-address-hint`
                : `${id}-address-hint`
            }
            className={TEXTAREA_CLASS}
          />
          {errors.address !== undefined && (
            <FieldError id={`${id}-address-error`} message={errors.address} />
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

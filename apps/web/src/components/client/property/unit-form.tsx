"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type FormEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { toFormError } from "@/lib/api/form-error";
import type { Unit } from "@/lib/api/generated/model";
import {
  getGetPropertyQueryKey,
  getListPropertiesQueryKey,
  getListUnitsQueryKey,
  useCreateUnit,
  useUpdateUnit,
} from "@/lib/api/generated/properties/properties";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  UNIT_LABEL_MAX_LENGTH,
  checkUnitForm,
} from "./utils";
import type { UnitFormErrors } from "./type";

type UnitFormProps = {
  orgId: string;
  propertyId: string;
  propertyName: string;
  // The unit being changed; none when adding.
  unit: Unit | undefined;
  onSaved: (unit: Unit) => void;
};

// The fields the API's errors can name; any other field goes to the alert.
const FORM_FIELDS = ["label"];

// "Tambah unit" and "Ubah unit": the unit's name. The save button sits outside the form, in the
// panel's footer, and submits it through its `form` attribute.
export function UnitForm({
  orgId,
  propertyId,
  propertyName,
  unit,
  onSaved,
}: UnitFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const labelRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const [label, setLabel] = useState(unit?.label ?? "");
  const [errors, setErrors] = useState<UnitFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

  // A new unit changes the property's unit count and occupancy as well.
  const invalidateUnits = () =>
    queryClient.invalidateQueries({
      queryKey: getListUnitsQueryKey(orgId, propertyId),
    });
  const invalidateProperties = () =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: getListPropertiesQueryKey(orgId),
      }),
      queryClient.invalidateQueries({
        queryKey: getGetPropertyQueryKey(orgId, propertyId),
      }),
    ]);
  const create = useCreateUnit({
    mutation: {
      onSuccess: () => Promise.all([invalidateUnits(), invalidateProperties()]),
    },
  });
  const update = useUpdateUnit({ mutation: { onSuccess: invalidateUnits } });
  const saving = create.isPending || update.isPending;

  function clearError(field: keyof UnitFormErrors) {
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, FORM_FIELDS);
    if (failed.field === "label") {
      setErrors({ label: failed.message });
      labelRef.current?.focus();
    } else {
      setAlert(failed.message);
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkUnitForm({ label });
    if (!checked.ok) {
      setErrors(checked.errors);
      labelRef.current?.focus();
      return;
    }
    setErrors({});

    const options = { onSuccess: onSaved, onError: handleError };
    if (unit === undefined) {
      create.mutate({ orgId, propertyId, data: checked.input }, options);
    } else {
      update.mutate({ orgId, unitId: unit.id, data: checked.input }, options);
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
          <Label htmlFor={`${id}-label`} className={FIELD_LABEL_CLASS}>
            Nama unit
          </Label>
          <Input
            ref={labelRef}
            id={`${id}-label`}
            name="label"
            type="text"
            autoComplete="off"
            maxLength={UNIT_LABEL_MAX_LENGTH}
            placeholder="Contoh: Kamar 11"
            value={label}
            onChange={(event) => {
              setLabel(event.target.value);
              clearError("label");
            }}
            disabled={saving}
            aria-invalid={errors.label !== undefined}
            aria-describedby={
              errors.label !== undefined
                ? `${id}-label-error ${id}-label-hint`
                : `${id}-label-hint`
            }
            className={INPUT_CLASS}
          />
          {errors.label !== undefined && (
            <FieldError id={`${id}-label-error`} message={errors.label} />
          )}
          <p id={`${id}-label-hint`} className={FIELD_HINT_CLASS}>
            Harus beda dari unit lain di {propertyName}.
            <span className="hidden md:inline">
              {" "}
              Huruf besar dan kecil dianggap sama.
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
          {saving ? "Menyimpan…" : "Simpan unit"}
        </Button>
      </div>
    </>
  );
}

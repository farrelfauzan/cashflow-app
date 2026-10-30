"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import type { PropertyKind, TermMonths, Unit } from "@/lib/api/generated/model";
import {
  getGetPropertyQueryKey,
  getListPropertiesQueryKey,
  getListUnitPricesQueryKey,
  getListUnitsQueryKey,
  useCreateUnit,
  useListUnitPrices,
  useUpdateUnit,
} from "@/lib/api/generated/properties/properties";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
  UNIT_FORM_API_FIELDS,
  UNIT_LABEL_MAX_LENGTH,
  initialUnitFormValues,
  lastTermPrices,
  unitFormSchema,
} from "./utils";

import { UnitPriceHistory } from "./unit-price-history";
import { UnitTermsField } from "./unit-terms-field";

type UnitFormProps = {
  orgId: string;
  propertyId: string;
  propertyName: string;
  propertyKind: PropertyKind;
  // The unit being changed; none when adding.
  unit: Unit | undefined;
  // "Today" in Jakarta, read once when the panel opened.
  today: string;
  // The line at the top after "Ubah harga" saved a price; "" for none.
  notice: string;
  onSaved: (unit: Unit) => void;
  // "Ubah harga" on one of the unit's priced terms.
  onChangePrice: (months: TermMonths) => void;
};

// "Tambah unit" and "Ubah unit": the unit's name, the terms it offers with their prices, and when
// changing one, its price history. The save button sits outside the form, in the panel's footer,
// and submits it through its `form` attribute.
export function UnitForm({
  orgId,
  propertyId,
  propertyName,
  propertyKind,
  unit,
  today,
  notice,
  onSaved,
  onChangePrice,
}: UnitFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: unitFormSchema(unit),
    defaultValues: initialUnitFormValues(unit, propertyKind),
    apiFields: UNIT_FORM_API_FIELDS,
  });
  const labelError = form.formState.errors.label?.message;
  // The same query as "Riwayat harga": a term the unit no longer offers shows its last price.
  const history = useListUnitPrices(orgId, unit?.id ?? "", {
    query: { enabled: unit !== undefined },
  });
  const lastPrices = lastTermPrices(history.data ?? [], today);

  // A new unit changes the property's unit count and occupancy as well. A price typed for a term
  // adds a row to the unit's price history.
  const invalidateUnits = (saved: Unit) =>
    Promise.all([
      queryClient.invalidateQueries({
        queryKey: getListUnitsQueryKey(orgId, propertyId),
      }),
      queryClient.invalidateQueries({
        queryKey: getListUnitPricesQueryKey(orgId, saved.id),
      }),
    ]);
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
      onSuccess: (saved) =>
        Promise.all([invalidateUnits(saved), invalidateProperties()]),
    },
  });
  const update = useUpdateUnit({
    mutation: { onSuccess: (saved) => invalidateUnits(saved) },
  });
  const saving = create.isPending || update.isPending;

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    const options = { onSuccess: onSaved, onError: showApiError };
    if (unit === undefined) {
      create.mutate({ orgId, propertyId, data: input }, options);
    } else {
      update.mutate({ orgId, unitId: unit.id, data: input }, options);
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
        <p
          role="status"
          aria-live="polite"
          className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
        >
          {notice}
        </p>
        {alert !== undefined && <FormAlert message={alert} />}

        <div className="flex flex-col gap-2">
          <Label htmlFor={`${id}-label`} className={FIELD_LABEL_CLASS}>
            Nama unit
          </Label>
          <Input
            id={`${id}-label`}
            type="text"
            autoComplete="off"
            maxLength={UNIT_LABEL_MAX_LENGTH}
            placeholder="Contoh: Kamar 11"
            {...field("label")}
            disabled={saving}
            aria-invalid={labelError !== undefined}
            aria-describedby={
              labelError !== undefined
                ? `${id}-label-error ${id}-label-hint`
                : `${id}-label-hint`
            }
            className={INPUT_CLASS}
          />
          {labelError !== undefined && (
            <FieldError id={`${id}-label-error`} message={labelError} />
          )}
          <p id={`${id}-label-hint`} className={FIELD_HINT_CLASS}>
            Harus beda dari unit lain di {propertyName}.
            <span className="hidden md:inline">
              {" "}
              Huruf besar dan kecil dianggap sama.
            </span>
          </p>
        </div>

        <UnitTermsField
          form={form}
          unit={unit}
          lastPrices={lastPrices}
          disabled={saving}
          onChangePrice={onChangePrice}
        />

        {unit !== undefined && (
          <UnitPriceHistory orgId={orgId} unitId={unit.id} today={today} />
        )}
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

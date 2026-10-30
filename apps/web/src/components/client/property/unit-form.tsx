"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type FormEvent } from "react";
import { flushSync } from "react-dom";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { SheetClose } from "@/components/ui/sheet";
import { toFormError } from "@/lib/api/form-error";
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
  UNIT_LABEL_MAX_LENGTH,
  checkUnitForm,
  initialCustomTermRow,
  initialTermRows,
  lastTermPrices,
} from "./utils";
import type { UnitCustomTermRow, UnitFormErrors, UnitTermRow } from "./type";

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

// The fields the API's errors can name; any other field goes to the alert.
const FORM_FIELDS = ["label", "terms"];

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
  const labelRef = useRef<HTMLInputElement>(null);
  const termsRef = useRef<HTMLFieldSetElement>(null);
  const queryClient = useQueryClient();
  const [label, setLabel] = useState(unit?.label ?? "");
  const [terms, setTerms] = useState<UnitTermRow[]>(() =>
    initialTermRows(unit, propertyKind),
  );
  const [custom, setCustom] = useState<UnitCustomTermRow>(() =>
    initialCustomTermRow(unit),
  );
  const [errors, setErrors] = useState<UnitFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);
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

  function clearError(field: keyof UnitFormErrors) {
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  // A field with an error first ("Jumlah bulan" or a price), else the first checkbox. Call it
  // once the errors are rendered (flushSync), so aria-invalid is already on the field.
  function focusTerms() {
    const fieldset = termsRef.current;
    const target =
      fieldset?.querySelector<HTMLElement>('input[aria-invalid="true"]') ??
      fieldset?.querySelector<HTMLElement>("button");
    target?.focus();
  }

  function changeRow(changed: UnitTermRow) {
    setTerms((current) =>
      current.map((row) => (row.months === changed.months ? changed : row)),
    );
    setErrors((current) => {
      const next = { ...current };
      delete next.terms;
      if (next.prices !== undefined) {
        const prices = { ...next.prices };
        delete prices[changed.months];
        next.prices = prices;
      }
      return next;
    });
  }

  function changeCustom(changed: UnitCustomTermRow) {
    setCustom(changed);
    setErrors((current) => {
      const next = { ...current };
      delete next.terms;
      delete next.customMonths;
      delete next.customPrice;
      return next;
    });
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, FORM_FIELDS);
    if (failed.field === "label") {
      setErrors({ label: failed.message });
      labelRef.current?.focus();
    } else if (failed.field === "terms") {
      setErrors({ terms: failed.message });
      focusTerms();
    } else {
      setAlert(failed.message);
    }
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkUnitForm({ label, terms, custom }, unit);
    if (!checked.ok) {
      flushSync(() => setErrors(checked.errors));
      if (checked.errors.label !== undefined) labelRef.current?.focus();
      else focusTerms();
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
        <p
          role="status"
          aria-live="polite"
          className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
        >
          {notice}
        </p>
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

        <UnitTermsField
          fieldsetRef={termsRef}
          unit={unit}
          rows={terms}
          custom={custom}
          error={errors.terms}
          priceErrors={errors.prices}
          customMonthsError={errors.customMonths}
          customPriceError={errors.customPrice}
          lastPrices={lastPrices}
          disabled={saving}
          onRowChange={changeRow}
          onCustomChange={changeCustom}
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

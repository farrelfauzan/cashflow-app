"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState, type FormEvent } from "react";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import type { TermMonths, Unit, UnitPrice } from "@/lib/api/generated/model";
import {
  getListUnitPricesQueryKey,
  getListUnitsQueryKey,
  useCreateUnitPrice,
} from "@/lib/api/generated/properties/properties";
import type { UnitPriceFormErrors } from "./type";
import {
  checkUnitPriceForm,
  currentPriceText,
  PRICE_AMOUNT_HINT,
  PRICE_DATE_HINT,
  priceTermOptions,
  UNIT_PRICE_API_FIELDS,
} from "./utils";

type UnitPriceFormProps = {
  orgId: string;
  propertyId: string;
  unit: Unit;
  // The term whose "Ubah harga" was pressed.
  months: TermMonths;
  // "Today" in Jakarta, read once when the panel opened.
  today: string;
  onSaved: (price: UnitPrice) => void;
  // "Kembali": back to the unit's form, which keeps what was typed there.
  onBack: () => void;
};

// "Ubah harga": a new price for one of the unit's terms from a date, today or later. Prices are
// never changed; this adds a row to the history. Its own footer holds "Kembali" and "Simpan
// harga".
export function UnitPriceForm({
  orgId,
  propertyId,
  unit,
  months: initialMonths,
  today,
  onSaved,
  onBack,
}: UnitPriceFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const monthsRef = useRef<HTMLButtonElement>(null);
  const amountRef = useRef<HTMLInputElement>(null);
  const dateRef = useRef<HTMLInputElement>(null);
  const queryClient = useQueryClient();
  const [months, setMonths] = useState<TermMonths>(initialMonths);
  const [amountText, setAmountText] = useState("");
  const [effectiveFrom, setEffectiveFrom] = useState(today);
  const [errors, setErrors] = useState<UnitPriceFormErrors>({});
  const [alert, setAlert] = useState<string | null>(null);

  // The unit's terms (price in force, next price) and its history both change.
  const create = useCreateUnitPrice({
    mutation: {
      onSuccess: () =>
        Promise.all([
          queryClient.invalidateQueries({
            queryKey: getListUnitsQueryKey(orgId, propertyId),
          }),
          queryClient.invalidateQueries({
            queryKey: getListUnitPricesQueryKey(orgId, unit.id),
          }),
        ]),
    },
  });
  const saving = create.isPending;
  const options = priceTermOptions(unit);

  function clearError(field: keyof UnitPriceFormErrors) {
    setErrors((current) => {
      const next = { ...current };
      delete next[field];
      return next;
    });
  }

  function focusFirst(found: UnitPriceFormErrors) {
    if (found.months !== undefined) monthsRef.current?.focus();
    else if (found.amount !== undefined) amountRef.current?.focus();
    else if (found.effectiveFrom !== undefined) dateRef.current?.focus();
  }

  function handleError(error: unknown) {
    const failed = toFormError(error, UNIT_PRICE_API_FIELDS);
    if (failed.field === null) {
      setAlert(failed.message);
      return;
    }
    const found: UnitPriceFormErrors =
      failed.field === "months"
        ? { months: failed.message }
        : failed.field === "amount"
          ? { amount: failed.message }
          : { effectiveFrom: failed.message };
    setErrors(found);
    focusFirst(found);
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (saving) return;

    setAlert(null);
    const checked = checkUnitPriceForm(
      { months, amountText, effectiveFrom },
      today,
    );
    if (!checked.ok) {
      setErrors(checked.errors);
      focusFirst(checked.errors);
      return;
    }
    setErrors({});
    create.mutate(
      { orgId, unitId: unit.id, data: checked.input },
      { onSuccess: onSaved, onError: handleError },
    );
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

        <SelectField
          triggerRef={monthsRef}
          label="Masa sewa"
          value={String(months)}
          placeholder=""
          options={options}
          onValueChange={(value) => {
            const picked = unit.terms.find(
              (term) => String(term.months) === value,
            );
            if (picked !== undefined) setMonths(picked.months);
            clearError("months");
          }}
          hint={currentPriceText(unit, months)}
          error={errors.months}
          disabled={saving}
        />

        <RupiahField
          inputRef={amountRef}
          label="Harga baru"
          name="amount"
          placeholder="0"
          hint={PRICE_AMOUNT_HINT}
          value={amountText}
          onChange={(text) => {
            setAmountText(text);
            clearError("amount");
          }}
          error={errors.amount}
          disabled={saving}
        />

        <DateField
          inputRef={dateRef}
          label="Berlaku mulai"
          name="effectiveFrom"
          value={effectiveFrom}
          onChange={(date) => {
            setEffectiveFrom(date);
            clearError("effectiveFrom");
          }}
          hint={PRICE_DATE_HINT}
          error={errors.effectiveFrom}
          disabled={saving}
        />
      </form>
      <div className="flex gap-2.5 border-t px-6 py-4">
        <Button
          type="button"
          variant="outline"
          onClick={onBack}
          disabled={saving}
          className="h-13 flex-1 rounded-xl border-[1.5px] bg-card text-[17px] font-semibold"
        >
          Kembali
        </Button>
        <Button
          type="submit"
          form={formId}
          disabled={saving}
          className="h-13 flex-2 rounded-xl px-3 text-[17px] font-semibold whitespace-normal"
        >
          {saving ? "Menyimpan…" : "Simpan harga"}
        </Button>
      </div>
    </>
  );
}

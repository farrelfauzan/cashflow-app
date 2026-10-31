"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId } from "react";
import { Controller } from "react-hook-form";

import { DateField } from "@/components/client/form/date-field";
import { FormAlert } from "@/components/client/form/form-alert";
import { RupiahField } from "@/components/client/form/rupiah-field";
import { SelectField } from "@/components/client/form/select-field";
import { useApiForm } from "@/components/client/form/use-api-form";
import { Button } from "@/components/ui/button";
import type { TermMonths, Unit, UnitPrice } from "@/lib/api/generated/model";
import {
  getListUnitPricesQueryKey,
  getListUnitsQueryKey,
  useCreateUnitPrice,
} from "@/lib/api/generated/properties/properties";
import {
  currentPriceText,
  PRICE_AMOUNT_HINT,
  PRICE_DATE_HINT,
  priceTermOptions,
  UNIT_PRICE_FORM_API_FIELDS,
  unitPriceFormSchema,
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
  months,
  today,
  onSaved,
  onBack,
}: UnitPriceFormProps) {
  const id = useId();
  const formId = `${id}-form`;
  const queryClient = useQueryClient();
  const { form, showApiError, alert } = useApiForm({
    schema: unitPriceFormSchema(today),
    defaultValues: { months, amount: "", effectiveFrom: today },
    apiFields: UNIT_PRICE_FORM_API_FIELDS,
  });

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

  const submit = form.handleSubmit((input) => {
    if (saving) return;
    create.mutate(
      { orgId, unitId: unit.id, data: input },
      { onSuccess: onSaved, onError: showApiError },
    );
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

        <Controller
          control={form.control}
          name="months"
          render={({ field, fieldState }) => (
            <SelectField
              triggerRef={field.ref}
              label="Masa sewa"
              value={String(field.value)}
              placeholder=""
              options={options}
              onValueChange={(value) => {
                const picked = unit.terms.find(
                  (term) => String(term.months) === value,
                );
                if (picked !== undefined) field.onChange(picked.months);
                form.clearErrors("months");
              }}
              hint={currentPriceText(unit, field.value)}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />

        <Controller
          control={form.control}
          name="amount"
          render={({ field, fieldState }) => (
            <RupiahField
              inputRef={field.ref}
              label="Harga baru"
              name="amount"
              placeholder="0"
              hint={PRICE_AMOUNT_HINT}
              value={field.value}
              onChange={(text) => {
                field.onChange(text);
                form.clearErrors("amount");
              }}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
        />

        <Controller
          control={form.control}
          name="effectiveFrom"
          render={({ field, fieldState }) => (
            <DateField
              inputRef={field.ref}
              label="Berlaku mulai"
              name="effectiveFrom"
              value={field.value}
              onChange={(date) => {
                field.onChange(date);
                form.clearErrors("effectiveFrom");
              }}
              hint={PRICE_DATE_HINT}
              error={fieldState.error?.message}
              disabled={saving}
            />
          )}
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

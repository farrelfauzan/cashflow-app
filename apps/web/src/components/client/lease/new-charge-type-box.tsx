"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, type KeyboardEvent } from "react";
import { Controller } from "react-hook-form";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import { useApiForm } from "@/components/client/form/use-api-form";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
} from "@/components/client/form/utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import type { ChargeType } from "@/lib/api/generated/model";
import {
  getListChargeTypesQueryKey,
  useCreateChargeType,
} from "@/lib/api/generated/tenants/tenants";
import {
  CHARGE_TYPE_API_FIELDS,
  CHARGE_TYPE_METERED_LABEL,
  CHARGE_TYPE_NAME_LABEL,
  CHARGE_TYPE_NAME_MAX_LENGTH,
  CHARGE_TYPE_NAME_PLACEHOLDER,
  CREATE_CHARGE_TYPE_LABEL,
  chargeTypeFormSchema,
  NEW_CHARGE_TYPE_HINT,
  NEW_CHARGE_TYPE_TITLE,
} from "./utils";

type NewChargeTypeBoxProps = {
  orgId: string;
  // The panel is saving.
  disabled: boolean;
  // Called once the type is added and the list of types is loaded again.
  onAdded: (type: ChargeType) => void;
};

// "Jenis biaya baru" inside "Biaya tambahan": a name and whether it is metered, added for the
// whole organization. It sits inside the panel's form, so it is not a <form> of its own (its
// react-hook-form form is submitted by hand): its button is type="button", and Enter in the name
// adds the type instead of saving the panel.
export function NewChargeTypeBox({
  orgId,
  disabled,
  onAdded,
}: NewChargeTypeBoxProps) {
  const id = useId();
  const titleId = `${id}-title`;
  const errorId = `${id}-name-error`;
  const queryClient = useQueryClient();
  const { form, field, showApiError, alert } = useApiForm({
    schema: chargeTypeFormSchema,
    defaultValues: { name: "", metered: false },
    apiFields: CHARGE_TYPE_API_FIELDS,
  });
  const error = form.formState.errors.name?.message;

  const create = useCreateChargeType();
  const adding = create.isPending;
  const busy = disabled || adding;

  // The name gets focus when the box opens; a refused add focuses it again (useApiForm).
  useEffect(() => {
    form.setFocus("name");
  }, [form]);

  const add = form.handleSubmit((input) => {
    if (busy) return;
    create.mutate(
      { orgId, data: input },
      {
        onSuccess: async (type) => {
          await queryClient.invalidateQueries({
            queryKey: getListChargeTypesQueryKey(orgId),
          });
          onAdded(type);
        },
        onError: showApiError,
      },
    );
  });

  function handleNameKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter") return;
    event.preventDefault();
    if (!busy) void add();
  }

  return (
    <div
      role="group"
      aria-labelledby={titleId}
      className="flex flex-col gap-3 rounded-xl border-[1.5px] border-input p-3.5"
    >
      <p id={titleId} className="text-base font-bold md:text-[17px]">
        {NEW_CHARGE_TYPE_TITLE}
      </p>
      {alert !== undefined && <FormAlert message={alert} />}
      <div className="flex flex-col gap-2">
        <Label htmlFor={`${id}-name`} className={FIELD_LABEL_CLASS}>
          {CHARGE_TYPE_NAME_LABEL}
        </Label>
        <Input
          {...field("name")}
          id={`${id}-name`}
          type="text"
          autoComplete="off"
          maxLength={CHARGE_TYPE_NAME_MAX_LENGTH}
          placeholder={CHARGE_TYPE_NAME_PLACEHOLDER}
          onKeyDown={handleNameKeyDown}
          disabled={busy}
          aria-invalid={error !== undefined}
          aria-describedby={error !== undefined ? errorId : undefined}
          className={INPUT_CLASS}
        />
        {error !== undefined && <FieldError id={errorId} message={error} />}
      </div>
      <Label
        htmlFor={`${id}-metered`}
        className="min-h-11 gap-3 text-[15px] leading-normal font-semibold md:text-base"
      >
        <Controller
          control={form.control}
          name="metered"
          render={({ field: metered }) => (
            <Checkbox
              ref={metered.ref}
              id={`${id}-metered`}
              checked={metered.value}
              onCheckedChange={(checked) => metered.onChange(checked === true)}
              disabled={busy}
              className="size-[22px]"
            />
          )}
        />
        {CHARGE_TYPE_METERED_LABEL}
      </Label>
      <Button
        type="button"
        variant="outline"
        disabled={busy}
        onClick={() => {
          if (!busy) void add();
        }}
        className="h-12 rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold"
      >
        {adding ? "Menambahkan…" : CREATE_CHARGE_TYPE_LABEL}
      </Button>
      <p className={FIELD_HINT_CLASS}>{NEW_CHARGE_TYPE_HINT}</p>
    </div>
  );
}

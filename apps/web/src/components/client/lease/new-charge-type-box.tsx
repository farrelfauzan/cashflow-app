"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useId, useRef, useState, type KeyboardEvent } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { FormAlert } from "@/components/client/form/form-alert";
import {
  FIELD_HINT_CLASS,
  FIELD_LABEL_CLASS,
  INPUT_CLASS,
} from "@/components/client/form/utils";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { toFormError } from "@/lib/api/form-error";
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
  checkChargeTypeName,
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
// whole organization. It sits inside the panel's form, so it is not a form of its own: its button
// is type="button", and Enter in the name adds the type instead of saving the panel.
export function NewChargeTypeBox({
  orgId,
  disabled,
  onAdded,
}: NewChargeTypeBoxProps) {
  const id = useId();
  const titleId = `${id}-title`;
  const errorId = `${id}-name-error`;
  const queryClient = useQueryClient();
  const nameRef = useRef<HTMLInputElement>(null);
  const [name, setName] = useState("");
  const [metered, setMetered] = useState(false);
  const [error, setError] = useState<string | undefined>(undefined);
  const [alert, setAlert] = useState<string | null>(null);

  const create = useCreateChargeType();
  const adding = create.isPending;
  const busy = disabled || adding;

  // The name gets focus when the box opens, and again after a refused add.
  // Each request is handled once, as soon as the field is enabled.
  const [focusName, setFocusName] = useState<object>({});
  const focused = useRef<object | null>(null);
  useEffect(() => {
    if (busy || focused.current === focusName) return;
    focused.current = focusName;
    nameRef.current?.focus();
  }, [focusName, busy]);

  function add() {
    if (busy) return;
    const checked = checkChargeTypeName(name);
    if (!checked.ok) {
      setError(checked.error);
      setFocusName({});
      return;
    }
    setError(undefined);
    setAlert(null);
    create.mutate(
      { orgId, data: { name: checked.name, metered } },
      {
        onSuccess: async (type) => {
          await queryClient.invalidateQueries({
            queryKey: getListChargeTypesQueryKey(orgId),
          });
          onAdded(type);
        },
        onError: (failure) => {
          const failed = toFormError(failure, CHARGE_TYPE_API_FIELDS);
          if (failed.field !== null) {
            setError(failed.message);
            setFocusName({});
          } else {
            setAlert(failed.message);
          }
        },
      },
    );
  }

  function handleNameKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key !== "Enter") return;
    event.preventDefault();
    add();
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
      {alert !== null && <FormAlert message={alert} />}
      <div className="flex flex-col gap-2">
        <Label htmlFor={`${id}-name`} className={FIELD_LABEL_CLASS}>
          {CHARGE_TYPE_NAME_LABEL}
        </Label>
        <Input
          ref={nameRef}
          id={`${id}-name`}
          name="chargeTypeName"
          type="text"
          autoComplete="off"
          maxLength={CHARGE_TYPE_NAME_MAX_LENGTH}
          placeholder={CHARGE_TYPE_NAME_PLACEHOLDER}
          value={name}
          onChange={(event) => {
            setName(event.target.value);
            setError(undefined);
          }}
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
        <Checkbox
          id={`${id}-metered`}
          checked={metered}
          onCheckedChange={(checked) => setMetered(checked === true)}
          disabled={busy}
          className="size-[22px]"
        />
        {CHARGE_TYPE_METERED_LABEL}
      </Label>
      <Button
        type="button"
        variant="outline"
        disabled={busy}
        onClick={add}
        className="h-12 rounded-xl border-[1.5px] bg-card px-[18px] text-base font-semibold"
      >
        {adding ? "Menambahkan…" : CREATE_CHARGE_TYPE_LABEL}
      </Button>
      <p className={FIELD_HINT_CLASS}>{NEW_CHARGE_TYPE_HINT}</p>
    </div>
  );
}

"use client";

import { useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import type { Property } from "@/lib/api/generated/model";
import {
  SCOPED_ROLES,
  type RoleFormErrors,
  type RoleFormState,
} from "@/lib/member/role-form";

import { ScopedRoleField } from "./scoped-role-field";
import { SupervisorRoleRow } from "./supervisor-role-row";
import { SCOPED_ROLE_LABELS } from "./utils";

type RoleFieldsProps = {
  // "edit" shows the member's Supervisor role, which cannot change; "invite" says why there
  // is no Supervisor to choose.
  mode: "edit" | "invite";
  value: RoleFormState;
  onChange: (value: RoleFormState) => void;
  properties: readonly Property[];
  errors: RoleFormErrors;
  disabled: boolean;
};

// "Peran": Admin and Karyawan, each for the whole organization or some properties.
export function RoleFields({
  mode,
  value,
  onChange,
  properties,
  errors,
  disabled,
}: RoleFieldsProps) {
  const errorId = `${useId()}-roles-error`;

  return (
    <fieldset
      aria-describedby={errors.roles !== undefined ? errorId : undefined}
      className="flex flex-col gap-3"
    >
      <legend
        className={
          mode === "edit"
            ? "pb-2.5 text-[17px] font-bold"
            : "pb-2.5 text-base font-semibold"
        }
      >
        Peran
      </legend>
      {mode === "edit" && (
        <SupervisorRoleRow checked={value.supervisor.length > 0} />
      )}
      {SCOPED_ROLES.map((key) => (
        <ScopedRoleField
          key={key}
          label={SCOPED_ROLE_LABELS[key]}
          value={value[key]}
          onChange={(next) => onChange({ ...value, [key]: next })}
          properties={properties}
          error={errors[key]}
          disabled={disabled}
        />
      ))}
      {mode === "invite" && (
        <p className="text-[15px] leading-[1.45] text-muted-foreground">
          Supervisor hanya ditetapkan saat usaha dibuat.
        </p>
      )}
      {errors.roles !== undefined && (
        <FieldError id={errorId} message={errors.roles} />
      )}
    </fieldset>
  );
}

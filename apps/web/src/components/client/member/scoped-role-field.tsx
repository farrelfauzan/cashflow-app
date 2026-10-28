"use client";

import { useId } from "react";

import { FieldError } from "@/components/client/form/field-error";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import type { Property } from "@/lib/api/generated/model";
import { toggleProperty, type ScopedRoleState } from "@/lib/member/role-form";

type ScopedRoleFieldProps = {
  // "Admin" or "Karyawan".
  label: string;
  value: ScopedRoleState;
  onChange: (value: ScopedRoleState) => void;
  properties: readonly Property[];
  error: string | undefined;
  disabled: boolean;
};

// Each row is a whole label, so the touch target is at least 44px tall.
const OPTION_CLASS = "min-h-11 gap-2.5 text-base leading-normal font-normal";

// One role in the form: whether the member has it, and where (the whole organization, or the
// properties ticked below).
export function ScopedRoleField({
  label,
  value,
  onChange,
  properties,
  error,
  disabled,
}: ScopedRoleFieldProps) {
  const id = useId();
  const errorId = `${id}-error`;

  return (
    <div className="flex flex-col gap-1 rounded-xl border px-3.5 py-2">
      <Label
        htmlFor={`${id}-role`}
        className="min-h-11 gap-3 text-[17px] leading-normal font-semibold"
      >
        <Checkbox
          id={`${id}-role`}
          checked={value.selected}
          onCheckedChange={(checked) =>
            onChange({ ...value, selected: checked === true })
          }
          disabled={disabled}
          className="size-[22px]"
        />
        {label}
      </Label>
      {value.selected && (
        <div className="flex flex-col pl-[34px]">
          <RadioGroup
            aria-label={`Wilayah ${label}`}
            value={value.scope}
            onValueChange={(scope) =>
              onChange({
                ...value,
                scope: scope === "properties" ? "properties" : "organization",
              })
            }
            disabled={disabled}
            className="gap-0"
          >
            <Label htmlFor={`${id}-organization`} className={OPTION_CLASS}>
              <RadioGroupItem
                id={`${id}-organization`}
                value="organization"
                className="size-[22px]"
              />
              Seluruh usaha
            </Label>
            <Label htmlFor={`${id}-properties`} className={OPTION_CLASS}>
              <RadioGroupItem
                id={`${id}-properties`}
                value="properties"
                className="size-[22px]"
              />
              Hanya properti tertentu
            </Label>
          </RadioGroup>
          {value.scope === "properties" && (
            <div
              role="group"
              aria-label={`Properti untuk ${label}`}
              aria-describedby={error !== undefined ? errorId : undefined}
              className="flex flex-col pl-8"
            >
              {properties.length === 0 ? (
                <p className="py-2.5 text-[15px] text-muted-foreground">
                  Belum ada properti.
                </p>
              ) : (
                properties.map((property) => (
                  <Label
                    key={property.id}
                    htmlFor={`${id}-${property.id}`}
                    className={OPTION_CLASS}
                  >
                    <Checkbox
                      id={`${id}-${property.id}`}
                      checked={value.propertyIds.includes(property.id)}
                      onCheckedChange={(checked) =>
                        onChange({
                          ...value,
                          propertyIds: toggleProperty(
                            value.propertyIds,
                            property.id,
                            checked === true,
                          ),
                        })
                      }
                      disabled={disabled}
                      className="size-[22px]"
                    />
                    {property.name}
                  </Label>
                ))
              )}
            </div>
          )}
        </div>
      )}
      {error !== undefined && (
        <div className="pb-1.5">
          <FieldError id={errorId} message={error} />
        </div>
      )}
    </div>
  );
}

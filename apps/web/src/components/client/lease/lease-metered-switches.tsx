"use client";

import type { UseFormReturn } from "react-hook-form";

import type { LeaseChargesInput } from "@/lib/api/generated/model";
import type { LeaseChargesFormValues } from "./type";
import { METERED_CHARGES_NOTE, METERED_SECTION_TITLE } from "./utils";

import { LeaseMeteredRow } from "./lease-metered-row";

type LeaseMeteredSwitchesProps = {
  form: UseFormReturn<LeaseChargesFormValues, unknown, LeaseChargesInput>;
  // The organization's metered charge types, in the API's order.
  rows: readonly { chargeTypeId: string; name: string }[];
  disabled: boolean;
};

// "Dari meteran" under the fixed charges: every metered charge type as a switch, on when the
// lease expects its amount every month. The section shows nothing when the organization has no
// metered type.
export function LeaseMeteredSwitches({
  form,
  rows,
  disabled,
}: LeaseMeteredSwitchesProps) {
  if (rows.length === 0) return null;
  return (
    <section className="flex flex-col gap-1 pt-2">
      <h3 className="text-base font-bold md:text-[17px]">
        {METERED_SECTION_TITLE}
      </h3>
      <p className="pb-1 text-[15px] leading-normal md:text-base">
        {METERED_CHARGES_NOTE}
      </p>
      <div className="flex flex-col">
        {rows.map((row) => (
          <LeaseMeteredRow
            key={row.chargeTypeId}
            form={form}
            chargeTypeId={row.chargeTypeId}
            name={row.name}
            disabled={disabled}
          />
        ))}
      </div>
    </section>
  );
}

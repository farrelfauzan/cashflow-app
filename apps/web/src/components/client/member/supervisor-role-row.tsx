"use client";

import { useId } from "react";

import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";

type SupervisorRoleRowProps = {
  // The member is a Supervisor.
  checked: boolean;
};

// Supervisor in "Atur akses": shown, never changed here. Supervisors are set when the
// organization is created.
export function SupervisorRoleRow({ checked }: SupervisorRoleRowProps) {
  const id = useId();
  const checkboxId = `${id}-supervisor`;
  const noteId = `${id}-supervisor-note`;

  return (
    <div className="flex items-start gap-3 opacity-75">
      <Checkbox
        id={checkboxId}
        checked={checked}
        disabled
        aria-describedby={noteId}
        className="mt-0.5 size-[22px]"
      />
      <div className="flex flex-col gap-0.5">
        <Label
          htmlFor={checkboxId}
          className="text-[17px] leading-normal font-semibold"
        >
          Supervisor
        </Label>
        <span
          id={noteId}
          className="text-[15px] leading-[1.45] text-muted-foreground"
        >
          Hanya ditetapkan saat usaha dibuat.
        </span>
      </div>
    </div>
  );
}

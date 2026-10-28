import { cn } from "@/lib/utils";
import { FREE_LABEL, OCCUPIED_LABEL } from "./utils";
import type { UnitOccupancyView } from "./type";

type UnitTenantProps = {
  view: UnitOccupancyView;
  className?: string;
};

// Who is in a unit: the tenant's name as plain text (the tenant pages come later), or a chip
// that says the unit is free ("Kosong"), or taken without a name the member may read ("Terisi").
export function UnitTenant({ view, className }: UnitTenantProps) {
  if (view.kind === "tenant") {
    return <span className={className}>{view.name}</span>;
  }
  return (
    <span
      className={cn(
        "inline-flex min-h-7 items-center rounded-lg bg-secondary px-2.5 text-sm font-semibold text-foreground md:text-base",
        className,
      )}
    >
      {view.kind === "free" ? FREE_LABEL : OCCUPIED_LABEL}
    </span>
  );
}

import type { PropertyKind } from "@/lib/api/generated/model";
import { propertyKindLabel } from "./utils";

type PropertyKindChipProps = {
  kind: PropertyKind;
};

// "Kos", "Kontrakan", "Ruko" or "Lainnya".
export function PropertyKindChip({ kind }: PropertyKindChipProps) {
  return (
    <span className="inline-flex min-h-7 items-center rounded-lg bg-secondary px-2.5 text-sm font-semibold whitespace-nowrap text-foreground md:text-base">
      {propertyKindLabel(kind)}
    </span>
  );
}

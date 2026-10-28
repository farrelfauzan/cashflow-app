import { cn } from "cn";

import { orgInitials } from "@/lib/organization/org-initials";

type OrgTileProps = {
  name: string;
  // "soft" on the picker's rows, "solid" in the side menu.
  tone: "soft" | "solid";
  className?: string;
};

// An organization's two-letter tile. Decorative: the name is always written next to it.
export function OrgTile({ name, tone, className }: OrgTileProps) {
  return (
    <span
      aria-hidden="true"
      className={cn(
        "flex shrink-0 items-center justify-center font-bold",
        tone === "soft"
          ? "bg-accent text-primary"
          : "bg-primary text-primary-foreground",
        className,
      )}
    >
      {orgInitials(name)}
    </span>
  );
}

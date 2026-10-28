import { ChevronRight } from "lucide-react";
import Link from "next/link";

import type { Organization } from "@/lib/api/generated/model";
import { orgPath } from "@/lib/organization/org-menu";

import { OrgTile } from "./org-tile";

type OrganizationChoiceProps = {
  organization: Organization;
};

export function OrganizationChoice({ organization }: OrganizationChoiceProps) {
  return (
    <Link
      href={orgPath(organization.id)}
      className="flex min-h-[72px] items-center gap-3.5 rounded-[14px] border bg-card px-4 py-3 shadow-xs transition-colors hover:bg-accent/40 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:min-h-[88px] md:gap-[18px] md:rounded-2xl md:px-[22px] md:py-4"
    >
      <OrgTile
        name={organization.name}
        tone="soft"
        className="size-11 rounded-xl text-base md:size-[52px] md:rounded-[14px] md:text-[19px]"
      />
      <span className="flex-1 text-lg font-semibold md:text-[22px]">
        {organization.name}
      </span>
      <ChevronRight
        aria-hidden="true"
        className="size-[22px] text-muted-foreground md:size-6"
      />
    </Link>
  );
}

"use client";

import { ArrowLeftRight } from "lucide-react";
import { useRouter } from "next/navigation";
import { useId } from "react";

import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { orgPath } from "@/lib/org-path";

import { useSidebarCollapsed } from "./sidebar-collapsed-context";
import type { OrgSwitchOption } from "./type";

type OrgSwitchSelectProps = {
  currentOrgId: string;
  organizations: readonly OrgSwitchOption[];
};

// "Ganti usaha": the member's organizations, showing the current one. On the collapsed iPad
// rail it is an icon button that opens the same list. Choosing another opens
// its Beranda; OrgCacheScope then removes the previous organization's queries, as for any
// change of organization.
export function OrgSwitchSelect({
  currentOrgId,
  organizations,
}: OrgSwitchSelectProps) {
  const router = useRouter();
  const id = useId();
  const collapsed = useSidebarCollapsed();

  return (
    <div className="flex flex-col gap-1.5 px-1 pb-3 rail:items-center rail:px-0">
      <Label
        htmlFor={id}
        className="px-1 text-sm font-semibold text-muted-foreground md:text-base rail:sr-only"
      >
        Ganti usaha
      </Label>
      <Select
        value={currentOrgId}
        onValueChange={(orgId) => {
          if (orgId !== currentOrgId) router.push(orgPath(orgId));
        }}
      >
        <SelectTrigger
          id={id}
          title={collapsed ? "Ganti usaha" : undefined}
          className="w-full rounded-[10px] bg-card text-base font-semibold data-[size=default]:h-11 rail:size-11 rail:justify-center rail:p-0 rail:[&>svg:last-child]:hidden"
        >
          <ArrowLeftRight
            aria-hidden="true"
            className="hidden size-5 text-foreground rail:block"
          />
          {collapsed ? (
            // The trigger styles its direct SelectValue child (flex, line clamp), which would
            // undo sr-only on it, so the rail hides a wrapper instead.
            <span className="sr-only">
              <SelectValue />
            </span>
          ) : (
            <SelectValue />
          )}
        </SelectTrigger>
        <SelectContent position="popper">
          {organizations.map((organization) => (
            <SelectItem
              key={organization.id}
              value={organization.id}
              className="min-h-11 text-base"
            >
              {organization.name}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

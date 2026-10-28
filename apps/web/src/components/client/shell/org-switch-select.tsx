"use client";

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

import type { OrgSwitchOption } from "./type";

type OrgSwitchSelectProps = {
  currentOrgId: string;
  organizations: readonly OrgSwitchOption[];
};

// "Ganti usaha": the member's organizations, showing the current one. Choosing another opens
// its Beranda; OrgCacheScope then removes the previous organization's queries, as for any
// change of organization.
export function OrgSwitchSelect({
  currentOrgId,
  organizations,
}: OrgSwitchSelectProps) {
  const router = useRouter();
  const id = useId();

  return (
    <div className="flex flex-col gap-1.5 px-1 pb-3">
      <Label
        htmlFor={id}
        className="px-1 text-sm font-semibold text-muted-foreground md:text-base"
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
          className="w-full rounded-[10px] bg-card text-base font-semibold data-[size=default]:h-11"
        >
          <SelectValue />
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

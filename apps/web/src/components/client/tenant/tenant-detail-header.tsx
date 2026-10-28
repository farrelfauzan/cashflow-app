"use client";

import { ChevronLeft } from "lucide-react";
import Link from "next/link";

import { formatPhone } from "@/lib/format/phone";
import { orgPath } from "@/lib/org-path";

type TenantDetailHeaderProps = {
  orgId: string;
  name: string;
  // E.164, as the API sends it.
  phone: string;
};

// The back link, the tenant's name and their number. The board's "WhatsApp aktif" is left out:
// the API has no such field.
export function TenantDetailHeader({
  orgId,
  name,
  phone,
}: TenantDetailHeaderProps) {
  return (
    <header className="flex flex-col gap-1.5">
      <Link
        href={orgPath(orgId, "tenants")}
        className="-ml-1 inline-flex min-h-11 items-center gap-1 self-start rounded-md pr-2 text-[15px] font-semibold text-primary hover:underline focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none md:text-base"
      >
        <ChevronLeft
          aria-hidden="true"
          className="size-[18px]"
          strokeWidth={2.2}
        />
        Penyewa
      </Link>
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        {name}
      </h1>
      <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
        {formatPhone(phone)}
      </p>
    </header>
  );
}

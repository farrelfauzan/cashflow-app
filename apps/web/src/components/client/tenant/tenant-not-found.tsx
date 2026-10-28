"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import { orgPath } from "@/lib/org-path";

type TenantNotFoundProps = {
  orgId: string;
};

// The tenant in the address is not one the member may see: it does not exist (the API answers
// 404), or none of their leases is within the member's reach (403). The screen says the same for
// both, so it never tells a member which tenants exist.
export function TenantNotFound({ orgId }: TenantNotFoundProps) {
  return (
    <div className="flex flex-col items-start gap-4">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          Penyewa tidak ditemukan
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          Penyewa ini tidak ada, atau tidak ditugaskan kepada Anda.
        </p>
      </div>
      <Button
        asChild
        variant="outline"
        className="h-12 rounded-xl border-[1.5px] bg-card px-5 text-base font-semibold"
      >
        <Link href={orgPath(orgId, "tenants")}>Kembali ke daftar penyewa</Link>
      </Button>
    </div>
  );
}

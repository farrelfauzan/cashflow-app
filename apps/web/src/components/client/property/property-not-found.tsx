import Link from "next/link";

import { Button } from "@/components/ui/button";
import { orgPath } from "@/lib/org-path";

type PropertyNotFoundProps = {
  orgId: string;
};

// The property in the address is not one the member may see: it does not exist, or it is not
// assigned to them. Both read the same, so a limited member never learns which.
export function PropertyNotFound({ orgId }: PropertyNotFoundProps) {
  return (
    <div className="flex flex-col items-start gap-4">
      <div className="flex flex-col gap-1.5">
        <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
          Properti tidak ditemukan
        </h1>
        <p className="text-[15px] leading-normal text-muted-foreground md:text-[17px]">
          Properti ini tidak ada, atau tidak ditugaskan kepada Anda.
        </p>
      </div>
      <Button
        asChild
        variant="outline"
        className="h-12 rounded-xl border-[1.5px] bg-card px-5 text-base font-semibold"
      >
        <Link href={orgPath(orgId, "properties")}>
          Kembali ke daftar properti
        </Link>
      </Button>
    </div>
  );
}

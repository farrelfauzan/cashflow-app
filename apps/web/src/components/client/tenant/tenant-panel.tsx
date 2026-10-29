"use client";

import { useState, type ReactNode } from "react";

import { LeaseForm } from "@/components/client/lease/lease-form";
import { leaseFormSubtitle } from "@/components/client/lease/utils";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { cn } from "@/lib/utils";
import type { OpenTenantPanel, TenantPanelState } from "./type";
import {
  leaseSavedNotice,
  tenantAddedNotice,
  tenantSavedNotice,
} from "./utils";

import { TenantAdded } from "./tenant-added";
import { TenantForm } from "./tenant-form";

type TenantPanelProps = {
  orgId: string;
  // The subtitle while adding a tenant.
  organizationName: string;
  // "Today" in Jakarta, for the lease form.
  today: string;
  canManageLeases: boolean;
  panel: TenantPanelState;
  onPanelChange: (panel: TenantPanelState) => void;
  onNotice: (notice: string) => void;
};

// The side panel of the tenant screens (boards Penyewa-Tambah.dc.html and Sewa-Baru.dc.html):
// from the right on iPad, the whole screen on the phone. "Tambah penyewa" goes on to "Penyewa
// ditambahkan" and from there to "Sewakan unit" in the same panel. The lease form gets the wide
// panel, so "Mulai tagih dari" stays in view next to the other fields.
export function TenantPanel({
  orgId,
  organizationName,
  today,
  canManageLeases,
  panel,
  onPanelChange,
  onNotice,
}: TenantPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenTenantPanel | null>(null);
  if (panel.kind !== "closed" && panel !== shown) setShown(panel);
  const close = () => onPanelChange({ kind: "closed" });

  let title = "";
  let subtitle = "";
  let body: ReactNode = null;
  switch (shown?.kind) {
    case "add":
      title = "Tambah penyewa";
      subtitle = organizationName;
      body = (
        <TenantForm
          orgId={orgId}
          tenant={undefined}
          onSaved={(tenant) => onPanelChange({ kind: "added", tenant })}
        />
      );
      break;
    case "added":
      title = "Penyewa ditambahkan";
      subtitle = organizationName;
      body = (
        <TenantAdded
          name={shown.tenant.name}
          canManageLeases={canManageLeases}
          onLater={() => {
            onNotice(tenantAddedNotice(shown.tenant.name));
            close();
          }}
          onRent={() => onPanelChange({ kind: "lease", tenant: shown.tenant })}
        />
      );
      break;
    case "edit":
      title = "Ubah penyewa";
      subtitle = leaseFormSubtitle(shown.tenant.name, shown.tenant.phone);
      body = (
        <TenantForm
          key={shown.tenant.id}
          orgId={orgId}
          tenant={shown.tenant}
          onSaved={(tenant) => {
            onNotice(tenantSavedNotice(tenant.name));
            close();
          }}
        />
      );
      break;
    case "lease":
      title = "Sewakan unit";
      subtitle = leaseFormSubtitle(shown.tenant.name, shown.tenant.phone);
      body = (
        <LeaseForm
          key={shown.tenant.id}
          orgId={orgId}
          tenant={shown.tenant}
          today={today}
          onSaved={(lease) => {
            onNotice(leaseSavedNotice(lease, shown.tenant.name));
            close();
          }}
        />
      );
      break;
  }

  return (
    <Sheet
      open={panel.kind !== "closed"}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <SheetContent
        side="right"
        className={cn(
          "w-full gap-0 bg-card p-0 sm:max-w-none",
          shown?.kind === "lease"
            ? "md:w-[920px] md:max-w-full"
            : "md:w-[470px]",
        )}
      >
        <SheetHeader className="gap-1 border-b px-6 pt-6 pr-16 pb-4">
          <SheetTitle className="text-2xl font-bold">{title}</SheetTitle>
          <SheetDescription className="text-base text-muted-foreground">
            {subtitle}
          </SheetDescription>
        </SheetHeader>
        {body}
      </SheetContent>
    </Sheet>
  );
}

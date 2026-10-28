"use client";

import { useState, type ReactNode } from "react";

import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import type { Member, Property } from "@/lib/api/generated/model";
import { formatPhone } from "@/lib/format/phone";
import type { MemberPanelState } from "@/lib/member/panel-state";
import type { PropertyNames } from "@/lib/member/role-labels";

import { InviteMemberForm } from "./invite-member-form";
import { InviteResult } from "./invite-result";
import { MemberAccessForm } from "./member-access-form";

type MemberPanelProps = {
  orgId: string;
  organizationName: string;
  panel: MemberPanelState;
  members: readonly Member[];
  properties: readonly Property[];
  propertyNames: PropertyNames;
  roleGuide: ReactNode;
  onPanelChange: (panel: MemberPanelState) => void;
  onNotice: (notice: string) => void;
};

type OpenPanel = Exclude<MemberPanelState, { kind: "closed" }>;

// The side panel of the members screen (board Members.dc.html): from the right on iPad, the
// whole screen on the phone. One panel at a time; its save button is its primary action.
export function MemberPanel({
  orgId,
  organizationName,
  panel,
  members,
  properties,
  propertyNames,
  roleGuide,
  onPanelChange,
  onNotice,
}: MemberPanelProps) {
  // The last panel that was open stays rendered while the sheet slides away.
  const [shown, setShown] = useState<OpenPanel | null>(null);
  if (panel.kind !== "closed" && panel !== shown) setShown(panel);

  const accessMember =
    shown?.kind === "access"
      ? members.find((member) => member.id === shown.memberId)
      : undefined;
  const open =
    panel.kind !== "closed" &&
    (panel.kind !== "access" || accessMember !== undefined);
  const close = () => onPanelChange({ kind: "closed" });

  let title = "";
  let subtitle = "";
  let body: ReactNode = null;
  switch (shown?.kind) {
    case "access":
      if (accessMember !== undefined) {
        title = "Atur akses";
        subtitle = `${accessMember.name} · ${formatPhone(accessMember.phone)}`;
        body = (
          <MemberAccessForm
            key={accessMember.id}
            orgId={orgId}
            organizationName={organizationName}
            member={accessMember}
            properties={properties}
            onSaved={(member) => {
              onNotice(`Peran ${member.name} sudah disimpan.`);
              close();
            }}
          />
        );
      }
      break;
    case "invite":
      title = "Tambah anggota";
      subtitle = organizationName;
      body = (
        <InviteMemberForm
          orgId={orgId}
          properties={properties}
          onInvited={(member) => onPanelChange({ kind: "invited", member })}
        />
      );
      break;
    case "invited":
      title = "Anggota ditambahkan";
      subtitle = organizationName;
      body = (
        <InviteResult member={shown.member} propertyNames={propertyNames} />
      );
      break;
    case "roles":
      title = "Arti tiap peran";
      subtitle = "Peran bawaan yang sama di setiap usaha";
      body = (
        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {roleGuide}
        </div>
      );
      break;
  }

  return (
    <Sheet
      open={open}
      onOpenChange={(next) => {
        if (!next) close();
      }}
    >
      <SheetContent
        side="right"
        className="w-full gap-0 bg-card p-0 sm:max-w-none md:w-[470px]"
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

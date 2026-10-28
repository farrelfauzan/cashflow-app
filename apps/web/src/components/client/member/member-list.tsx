"use client";

import type { Member } from "@/lib/api/generated/model";
import { canManageMember, canResendInvitation } from "./utils";
import type { MemberPermissions, PropertyNames } from "./type";

import { MemberCard } from "./member-card";
import { MemberTableRow } from "./member-table-row";

type MemberListProps = {
  orgId: string;
  members: readonly Member[];
  // The signed-in member (getOrgMe.membershipId): their row says "Anda" and has no "Atur".
  ownMembershipId: string;
  permissions: MemberPermissions;
  propertyNames: PropertyNames;
  onManage: (memberId: string) => void;
};

// Every member of the organization, in the API's order: a table on iPad, cards on the phone
// (boards Members.dc.html and Members-Phone.dc.html). Only one of the two is displayed.
export function MemberList({
  orgId,
  members,
  ownMembershipId,
  permissions,
  propertyNames,
  onManage,
}: MemberListProps) {
  const rows = members.map((member) => ({
    member,
    isSelf: member.id === ownMembershipId,
    canManage: canManageMember(member, ownMembershipId, permissions),
    canResend: canResendInvitation(member, permissions),
  }));

  return (
    <>
      <section className="hidden overflow-hidden rounded-2xl border bg-card lg:block">
        <div
          aria-hidden="true"
          className="grid grid-cols-[240px_minmax(0,1fr)_200px_80px] items-center gap-4 border-b bg-muted/40 px-5 py-2.5 text-sm font-semibold text-muted-foreground"
        >
          <span>Nama</span>
          <span>Peran</span>
          <span>Status</span>
          <span />
        </div>
        <ul aria-label="Daftar anggota">
          {rows.map((row) => (
            <MemberTableRow
              key={row.member.id}
              orgId={orgId}
              propertyNames={propertyNames}
              onManage={onManage}
              {...row}
            />
          ))}
        </ul>
      </section>

      <ul
        aria-label="Daftar anggota"
        className="flex flex-col gap-2.5 lg:hidden"
      >
        {rows.map((row) => (
          <MemberCard
            key={row.member.id}
            orgId={orgId}
            propertyNames={propertyNames}
            onManage={onManage}
            {...row}
          />
        ))}
      </ul>
    </>
  );
}

"use client";

import { useAbility } from "@casl/react";
import { keepPreviousData } from "@tanstack/react-query";
import { useMemo, useState, type ReactNode } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { ListPagination } from "@/components/client/list/list-pagination";
import { useLastPageRedirect } from "@/components/client/list/use-last-page-redirect";
import { useListUrl } from "@/components/client/list/use-list-url";
import {
  PAGE_SIZE_PARAM,
  readPage,
  readPageSize,
} from "@/components/client/list/utils";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import {
  useGetOrgMe,
  useListMembers,
} from "@/lib/api/generated/members/members";
import { useListProperties } from "@/lib/api/generated/properties/properties";
import { memberPermissions, propertyNames } from "./utils";
import type { MemberPanelState } from "./type";
import type { AppAbility } from "@/lib/capabilities";

import { MemberList } from "./member-list";
import { MemberPanel } from "./member-panel";
import { MembersHeader } from "./members-header";

type MembersViewProps = {
  orgId: string;
  organizationName: string;
  // "Arti tiap peran", rendered on the server.
  roleGuide: ReactNode;
};

// The members screen: one page of the list (page and size in the URL), and the panels that
// change it. The panels get the members on the current page, since "Atur" opens from a visible
// row. What shows comes from the signed-in member's capabilities (getOrgMe); after every change
// the list query is invalidated and loads again from the API.
export function MembersView({
  orgId,
  organizationName,
  roleGuide,
}: MembersViewProps) {
  const orgMe = useGetOrgMe(orgId);
  const url = useListUrl();
  const page = readPage(url.searchParams);
  const pageSize = readPageSize(url.searchParams);
  const members = useListMembers(
    orgId,
    { page, pageSize },
    { query: { placeholderData: keepPreviousData } },
  );
  useLastPageRedirect(url, page, pageSize, members.data);
  const ability = useAbility<AppAbility>();
  const properties = useListProperties(
    orgId,
    { pageSize: 100 },
    { query: { enabled: ability.can("read", "property") } },
  );
  const [panel, setPanel] = useState<MemberPanelState>({ kind: "closed" });
  // A short confirmation after a panel closes, announced to screen readers.
  const [notice, setNotice] = useState("");

  const permissions = memberPermissions(ability);
  const names = useMemo(
    () => propertyNames(properties.data?.items ?? []),
    [properties.data],
  );

  function openPanel(next: MemberPanelState) {
    setNotice("");
    setPanel(next);
  }

  const failed = orgMe.isError ? orgMe.error : members.error;

  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <MembersHeader
        organizationName={organizationName}
        canInvite={permissions.canInvite}
        onInvite={() => openPanel({ kind: "invite" })}
        onShowRoles={() => openPanel({ kind: "roles" })}
      />

      <p
        role="status"
        aria-live="polite"
        className="text-[15px] font-semibold text-primary empty:hidden md:text-base"
      >
        {notice}
      </p>

      {failed !== null ? (
        <div className="flex flex-col items-start gap-3">
          <FormAlert message={toFormError(failed, []).message} />
          <Button
            type="button"
            variant="outline"
            className="font-semibold"
            onClick={() => {
              void orgMe.refetch();
              void members.refetch();
            }}
          >
            Coba lagi
          </Button>
        </div>
      ) : orgMe.data === undefined || members.data === undefined ? (
        <p className="text-base text-muted-foreground">Memuat…</p>
      ) : (
        <div
          aria-busy={members.isPlaceholderData}
          className="flex flex-col gap-4 aria-busy:opacity-60"
        >
          <MemberList
            orgId={orgId}
            members={members.data.items}
            ownMembershipId={orgMe.data.membershipId}
            permissions={permissions}
            propertyNames={names}
            onManage={(memberId) => openPanel({ kind: "access", memberId })}
          />
          <ListPagination
            page={page}
            pageSize={pageSize}
            total={members.data.total}
            hrefFor={url.hrefFor}
            onPageSizeChange={(size) => url.change({ [PAGE_SIZE_PARAM]: size })}
          />
        </div>
      )}

      <MemberPanel
        orgId={orgId}
        organizationName={organizationName}
        panel={panel}
        members={members.data?.items ?? []}
        properties={properties.data?.items ?? []}
        propertyNames={names}
        roleGuide={roleGuide}
        onPanelChange={setPanel}
        onNotice={setNotice}
      />
    </div>
  );
}

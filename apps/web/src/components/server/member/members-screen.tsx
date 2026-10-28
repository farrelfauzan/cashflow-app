import { MembersView } from "@/components/client/member/members-view";
import { getMe } from "@/lib/api/generated/auth/auth";

import { RoleGuide } from "./role-guide";

type MembersScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Anggota tim" (boards Members.dc.html and Members-Phone.dc.html). The organization's name
// comes from the server; the list, the member's capabilities and the panels live in the client
// view, so the list refreshes after every change. The shell around it has already sent anyone
// who is not a member of this organization back to the picker.
export async function MembersScreen({ params }: MembersScreenProps) {
  const { orgId } = await params;
  const me = await getMe();
  const organizationName =
    me.organizations.find((o) => o.id === orgId)?.name ?? "";

  return (
    <MembersView
      orgId={orgId}
      organizationName={organizationName}
      roleGuide={<RoleGuide />}
    />
  );
}

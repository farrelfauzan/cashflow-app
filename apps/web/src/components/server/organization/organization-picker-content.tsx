import { redirect } from "next/navigation";

import { getMe } from "@/lib/api/generated/auth/auth";
import { orgPath } from "@/lib/organization/org-menu";

import { NoOrganization } from "./no-organization";
import { OrganizationChoiceList } from "./organization-choice-list";
import { PickerHeader } from "./picker-header";

// One organization goes straight in; none shows how to get access; several are listed. The
// API lists only the organizations where the membership is active.
export async function OrganizationPickerContent() {
  const me = await getMe();
  const [only] = me.organizations;
  if (only !== undefined && me.organizations.length === 1) {
    redirect(orgPath(only.id));
  }

  return (
    <>
      <PickerHeader name={me.user.name} phone={me.user.phone} />
      {only === undefined ? (
        <NoOrganization />
      ) : (
        <OrganizationChoiceList organizations={me.organizations} />
      )}
    </>
  );
}

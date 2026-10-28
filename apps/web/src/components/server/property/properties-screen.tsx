import { PropertiesView } from "@/components/client/property/properties-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type PropertiesScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Properti" (boards Properti.html and Properti-Phone.html). The organization's name comes from
// the server; the list, the member's capabilities and the panel live in the client view, so the
// list refreshes after every change. The shell around it has already sent anyone who is not a
// member of this organization back to the picker.
export async function PropertiesScreen({ params }: PropertiesScreenProps) {
  const { orgId } = await params;
  const me = await getMe();
  const organizationName =
    me.organizations.find((o) => o.id === orgId)?.name ?? "";

  return <PropertiesView orgId={orgId} organizationName={organizationName} />;
}

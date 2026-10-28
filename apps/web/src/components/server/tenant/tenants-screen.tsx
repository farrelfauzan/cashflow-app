import { TenantsView } from "@/components/client/tenant/tenants-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type TenantsScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Penyewa" (boards Penyewa.dc.html and Penyewa-Phone.dc.html). The organization's name comes
// from the server; the list loads in the client view, one page at a time, with the page, search
// and filters in the URL. The shell around it has already sent anyone who is not a member of this
// organization back to the picker.
export async function TenantsScreen({ params }: TenantsScreenProps) {
  const { orgId } = await params;
  const me = await getMe();
  const organizationName =
    me.organizations.find((o) => o.id === orgId)?.name ?? "";

  return <TenantsView orgId={orgId} organizationName={organizationName} />;
}

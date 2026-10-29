import { TenantImportView } from "@/components/client/tenant-import/tenant-import-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type TenantImportScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Impor penyewa" (boards Impor*.dc.html). The organization's name comes from the server, for
// the done step; the import itself runs in the client view.
export async function TenantImportScreen({ params }: TenantImportScreenProps) {
  const { orgId } = await params;
  const me = await getMe();
  const organizationName =
    me.organizations.find((o) => o.id === orgId)?.name ?? "";

  return <TenantImportView orgId={orgId} organizationName={organizationName} />;
}

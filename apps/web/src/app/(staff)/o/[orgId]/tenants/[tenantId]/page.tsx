import { TenantDetailScreen } from "@/components/server/tenant/tenant-detail-screen";

export default function TenantDetailPage({
  params,
}: PageProps<"/o/[orgId]/tenants/[tenantId]">) {
  return <TenantDetailScreen params={params} />;
}

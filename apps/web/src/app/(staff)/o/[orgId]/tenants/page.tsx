import { TenantsScreen } from "@/components/server/tenant/tenants-screen";

export default function TenantsPage({
  params,
}: PageProps<"/o/[orgId]/tenants">) {
  return <TenantsScreen params={params} />;
}

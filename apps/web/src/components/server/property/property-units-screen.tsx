import { PropertyUnitsView } from "@/components/client/property/property-units-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type PropertyUnitsScreenProps = {
  params: Promise<{ orgId: string; propertyId: string }>;
};

// One property's units (boards Properti-Unit.html and Properti-Unit-Phone.html). The client view
// loads the property with getProperty and says so when the member may not see it.
export async function PropertyUnitsScreen({
  params,
}: PropertyUnitsScreenProps) {
  const { orgId, propertyId } = await params;
  const me = await getMe();
  const organizationName =
    me.organizations.find((o) => o.id === orgId)?.name ?? "";

  return (
    <PropertyUnitsView
      orgId={orgId}
      propertyId={propertyId}
      organizationName={organizationName}
    />
  );
}

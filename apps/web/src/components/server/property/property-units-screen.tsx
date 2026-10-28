import { PropertyUnitsView } from "@/components/client/property/property-units-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type PropertyUnitsScreenProps = {
  params: Promise<{ orgId: string; propertyId: string }>;
};

// One property's units (boards Properti-Unit.html and Properti-Unit-Phone.html). There is no
// "get one property" request: the client view finds the property in the list the member may see
// and says so when it is not there.
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

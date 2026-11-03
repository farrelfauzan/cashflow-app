import { InvoicesView } from "@/components/client/invoice/invoices-view";

type BillingScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Penagihan": the month's invoices. The list loads in the client view,
// one page at a time, with the month, page, search and filters in the URL. The shell around it
// has already sent anyone who is not a member of this organization back to the picker.
export async function BillingScreen({ params }: BillingScreenProps) {
  const { orgId } = await params;
  return <InvoicesView orgId={orgId} />;
}

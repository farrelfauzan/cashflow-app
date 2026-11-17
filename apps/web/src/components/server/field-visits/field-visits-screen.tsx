import { FieldVisitsView } from "@/components/client/field-visits/field-visits-view";

type FieldVisitsScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Kunjungan hari ini" (board HP-Kunjungan): the karyawan's visits of the day. One readable
// column at every size, like Tagihan hari ini; "Catat hasil" is a bottom sheet on the phone and
// the right-hand drawer from md up. The data loads in the client view, as on Tagihan hari ini,
// since its sheets reload it; the shell around it has already sent anyone who is not a member
// back to the picker.
export async function FieldVisitsScreen({ params }: FieldVisitsScreenProps) {
  const { orgId } = await params;
  return (
    <div className="mx-auto flex w-full max-w-xl flex-col gap-3 md:gap-4">
      <FieldVisitsView orgId={orgId} />
    </div>
  );
}

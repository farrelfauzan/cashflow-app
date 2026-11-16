import { FieldTodayView } from "@/components/client/field-today/field-today-view";

type FieldTodayScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Tagihan hari ini" (board HP-Tagihan): the karyawan's first screen. One readable column at every
// size, like Setor tunai. The data loads in the client view; the shell around it has already sent
// anyone who is not a member back to the picker.
export async function FieldTodayScreen({ params }: FieldTodayScreenProps) {
  const { orgId } = await params;
  return (
    <div className="mx-auto flex w-full max-w-xl flex-col gap-3 md:gap-4">
      <FieldTodayView orgId={orgId} />
    </div>
  );
}

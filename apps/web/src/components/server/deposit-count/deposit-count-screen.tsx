import { DepositCountHeader } from "@/components/client/deposit-count/deposit-count-header";
import { DepositCountView } from "@/components/client/deposit-count/deposit-count-view";
import { getMe } from "@/lib/api/generated/auth/auth";

type DepositCountScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Setoran tunai" (board W20b), under Penagihan: the cash karyawan handed over, waiting to be
// counted, and the deposits already counted. The signed-in user's id comes from the server, so a
// member's own deposit offers no "Hitung"; the lists and the drawer live in the client view. The
// shell around it has already sent anyone who is not a member back to the picker.
export async function DepositCountScreen({ params }: DepositCountScreenProps) {
  const { orgId } = await params;
  const me = await getMe();
  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <DepositCountHeader orgId={orgId} />
      <DepositCountView orgId={orgId} userId={me.user.id} />
    </div>
  );
}

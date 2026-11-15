import { CashDepositView } from "@/components/client/deposit/cash-deposit-view";

type CashDepositScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Setor tunai" (board HP-Setor): the karyawan's cash in hand, the deposits waiting for an admin
// and the ones counted. One readable column at every size. The data loads in the client view;
// the shell around it has already sent anyone who is not a member back to the picker.
export async function CashDepositScreen({ params }: CashDepositScreenProps) {
  const { orgId } = await params;
  return (
    <div className="mx-auto flex w-full max-w-xl flex-col gap-4 md:gap-[22px]">
      <h1 className="text-[26px] font-bold tracking-[-0.01em] md:text-[34px]">
        Setor tunai
      </h1>
      <CashDepositView orgId={orgId} />
    </div>
  );
}

import { CheckQueueHeader } from "@/components/client/check-queue/check-queue-header";
import { CheckQueueView } from "@/components/client/check-queue/check-queue-view";

type CheckQueueScreenProps = {
  params: Promise<{ orgId: string }>;
};

// "Antrean cek" under Penagihan: what the system could not settle on its own. The list, its
// filters and the drawers live in the client view. The shell around it has already sent anyone
// who is not a member back to the picker.
export async function CheckQueueScreen({ params }: CheckQueueScreenProps) {
  const { orgId } = await params;
  return (
    <div className="flex flex-col gap-4 md:gap-[22px]">
      <CheckQueueHeader orgId={orgId} />
      <CheckQueueView orgId={orgId} />
    </div>
  );
}

"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";

import { FormAlert } from "@/components/client/form/form-alert";
import { Button } from "@/components/ui/button";
import { toFormError } from "@/lib/api/form-error";
import type { Member, MemberStatus } from "@/lib/api/generated/model";
import {
  getListMembersQueryKey,
  useSetMemberStatus,
} from "@/lib/api/generated/members/members";

import { DisableMemberDialog } from "./disable-member-dialog";

type MemberStatusSectionProps = {
  orgId: string;
  organizationName: string;
  member: Member;
};

// "Hentikan akses" in "Atur akses": disables the member after a confirmation, or, for a
// disabled member, enables them again with the same roles. A secondary action, never the
// panel's primary one.
export function MemberStatusSection({
  orgId,
  organizationName,
  member,
}: MemberStatusSectionProps) {
  const headingId = useId();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const [alert, setAlert] = useState<string | null>(null);
  const setStatus = useSetMemberStatus({
    mutation: {
      onSuccess: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });
  const place = organizationName === "" ? "usaha ini" : organizationName;
  const disabled = member.status === "disabled";

  function change(status: MemberStatus) {
    if (setStatus.isPending) return;
    setAlert(null);
    setStatus.mutate(
      { orgId, memberId: member.id, data: { status } },
      {
        onSuccess: () => setConfirming(false),
        onError: (error) => {
          setConfirming(false);
          setAlert(toFormError(error, []).message);
        },
      },
    );
  }

  return (
    <section
      aria-labelledby={headingId}
      className="flex flex-col items-start gap-2 border-t pt-4"
    >
      <h3 id={headingId} className="text-[17px] font-bold">
        {disabled ? "Akses dihentikan" : "Hentikan akses"}
      </h3>
      <p
        aria-live="polite"
        className="text-[15px] leading-normal text-muted-foreground"
      >
        {disabled
          ? `${member.name} sedang nonaktif dan tidak bisa masuk ke ${place}. Perannya tetap tersimpan dan berlaku lagi saat diaktifkan.`
          : `${member.name} tidak bisa masuk lagi ke ${place}. Catatan kerjanya tetap tersimpan, dan aksesnya bisa diaktifkan lagi.`}
      </p>
      {alert !== null && <FormAlert message={alert} />}
      {disabled ? (
        <Button
          type="button"
          variant="outline"
          onClick={() => change("active")}
          disabled={setStatus.isPending}
          className="rounded-[10px] border-[1.5px] bg-card px-4 text-[15px] font-semibold"
        >
          {setStatus.isPending ? "Mengaktifkan…" : "Aktifkan lagi"}
        </Button>
      ) : (
        <DisableMemberDialog
          memberName={member.name}
          place={place}
          open={confirming}
          onOpenChange={setConfirming}
          pending={setStatus.isPending}
          onConfirm={() => change("disabled")}
        />
      )}
    </section>
  );
}

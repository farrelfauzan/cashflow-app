"use client";

import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import type { Member } from "@/lib/api/generated/model";
import {
  getListMembersQueryKey,
  useResendMemberInvitation,
} from "@/lib/api/generated/members/members";
import { resendError, RESENT_MESSAGE } from "./utils";
import type { Outcome } from "./type";
import { cn } from "@/lib/utils";

type ResendInvitationButtonProps = {
  orgId: string;
  member: Member;
  // "Kirim ulang undangan" on iPad, "Kirim ulang" on the phone.
  label: string;
  className?: string;
};

// Sends a member's WhatsApp invitation again and says how it went. Either way the list loads
// again, since a failed send also changes the invitation's state. A number that cannot receive
// WhatsApp is not offered another try (see resendError).
export function ResendInvitationButton({
  orgId,
  member,
  label,
  className,
}: ResendInvitationButtonProps) {
  const queryClient = useQueryClient();
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const [blocked, setBlocked] = useState(false);
  const resend = useResendMemberInvitation({
    mutation: {
      onSettled: () =>
        queryClient.invalidateQueries({
          queryKey: getListMembersQueryKey(orgId),
        }),
    },
  });

  function handleClick() {
    if (resend.isPending) return;
    setOutcome(null);
    resend.mutate(
      { orgId, memberId: member.id },
      {
        onSuccess: () => setOutcome({ message: RESENT_MESSAGE, failed: false }),
        onError: (error) => {
          const { message, canRetry } = resendError(error);
          setOutcome({ message, failed: true });
          if (!canRetry) setBlocked(true);
        },
      },
    );
  }

  return (
    <div className={cn("flex flex-col items-start", className)}>
      {!blocked && (
        <Button
          type="button"
          variant="link"
          onClick={handleClick}
          disabled={resend.isPending}
          aria-label={`Kirim ulang undangan ke ${member.name}`}
          className="h-11 px-1.5 text-[15px] font-semibold no-underline hover:underline"
        >
          {resend.isPending ? "Mengirim…" : label}
        </Button>
      )}
      <p
        role="status"
        aria-live="polite"
        className={cn(
          "px-1.5 text-sm empty:hidden",
          outcome?.failed ? "font-semibold text-destructive" : "text-primary",
        )}
      >
        {outcome?.message}
      </p>
    </div>
  );
}

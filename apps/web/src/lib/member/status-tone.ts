import type { MemberStatusTone } from "@/lib/member/member-status";

// The text color of each member status, from the theme tokens.
export const STATUS_TONE_CLASS: Record<MemberStatusTone, string> = {
  active: "text-primary",
  pending: "text-foreground/85",
  disabled: "text-muted-foreground",
};

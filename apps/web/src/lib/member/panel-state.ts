import type { Member } from "@/lib/api/generated/model";

// The side panel of the members screen; one at a time.
export type MemberPanelState =
  | { kind: "closed" }
  // "Atur akses" for one member. Only the id is kept: the member shown is always the one in
  // the latest list, so a change made here shows at once.
  | { kind: "access"; memberId: string }
  | { kind: "invite" }
  // "Anggota ditambahkan": the member inviteMember returned.
  | { kind: "invited"; member: Member }
  | { kind: "roles" };

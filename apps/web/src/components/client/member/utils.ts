import type { ScopedRoleKey } from "@/lib/member/role-form";

export const EMPTY_NAME_MESSAGE = "Isi nama anggota.";
export const EMPTY_PHONE_MESSAGE = "Isi nomor WhatsApp.";
// The API's limit (InviteMember.name maxLength).
export const NAME_MAX_LENGTH = 100;

export const INPUT_CLASS =
  "h-13 rounded-xl border-[1.5px] bg-card px-3.5 text-[17px] aria-invalid:border-2";

export const NO_PERMISSIONS = { canInvite: false, canManage: false };

export const RESENT_MESSAGE = "Undangan sudah dikirim ulang.";

// The chip's surface by role key, from the theme tokens. Styling only; what a member may do
// comes from capabilities. A custom role gets the plain surface.
export const ROLE_CHIP_TONE: Record<string, string> = {
  supervisor: "bg-secondary text-foreground",
  admin: "bg-accent text-primary",
  karyawan: "bg-warning/15 text-foreground",
};

export const SCOPED_ROLE_LABELS: Record<ScopedRoleKey, string> = {
  admin: "Admin",
  karyawan: "Karyawan",
};

// Each row is a whole label, so the touch target is at least 44px tall.
export const ROLE_OPTION_CLASS =
  "min-h-11 gap-2.5 text-base leading-normal font-normal";

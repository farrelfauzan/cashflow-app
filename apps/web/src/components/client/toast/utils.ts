import {
  AlertTriangleIcon,
  CheckCircle2Icon,
  CopyIcon,
  InfoIcon,
  SendIcon,
  XCircleIcon,
  type LucideIcon,
} from "lucide-react";
import type { ToastIcon, ToastTone } from "./type";

export const TOAST_CLOSE_LABEL = "Tutup notifikasi";
export const TOASTER_LABEL = "Notifikasi";

// How long a toast stays: about 4 seconds to read; a red one, which reports something removed or
// cancelled, 6 seconds.
export const TOAST_DURATION_MS = 4000;
export const DESTRUCTIVE_TOAST_DURATION_MS = 6000;

// Colors from the theme tokens: cream on accent 5.7:1, on error 6.6:1, on warning 4.5:1, cream on
// ink 13:1.
export const TOAST_TONES: Record<
  ToastTone,
  { className: string; icon: LucideIcon }
> = {
  create: {
    className: "bg-primary text-primary-foreground",
    icon: CheckCircle2Icon,
  },
  destructive: {
    className: "bg-destructive text-destructive-foreground",
    icon: XCircleIcon,
  },
  caution: {
    className: "bg-warning text-warning-foreground",
    icon: AlertTriangleIcon,
  },
  neutral: { className: "bg-foreground text-background", icon: InfoIcon },
};

export function toastDuration(tone: ToastTone): number {
  return tone === "destructive"
    ? DESTRUCTIVE_TOAST_DURATION_MS
    : TOAST_DURATION_MS;
}

// Icons a message can ask for instead of its tone's (ToastMessage.icon).
export const TOAST_ICONS: Record<ToastIcon, LucideIcon> = {
  copy: CopyIcon,
  send: SendIcon,
};

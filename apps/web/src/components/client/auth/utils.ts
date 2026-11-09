import type { LogoutButtonVariant } from "./type";

// "Keluar" per place it sits in.
export const LOGOUT_BUTTON_CLASS: Record<LogoutButtonVariant, string> = {
  outline: "font-semibold",
  menu: "justify-start px-2 font-semibold text-muted-foreground rail:justify-center rail:px-0",
  sheet:
    "min-h-14 w-full justify-start gap-3.5 rounded-none border-b px-1 text-[17px] font-semibold text-foreground has-[>svg]:px-1 [&>svg:first-child]:size-[22px]! [&_svg]:text-muted-foreground",
};

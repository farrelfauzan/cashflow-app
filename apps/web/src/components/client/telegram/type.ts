// What a failed linkMyTelegram shows: no bot here after all (the menu item goes), or a message in
// the dialog.
export type StaffTelegramErrorView =
  { kind: "hidden" } | { kind: "alert"; message: string };

// "menu" at the foot of the side menu, "sheet" a 56px row in the phone's "Lainnya" (board
// HP-Lainnya).
export type TelegramMenuItemVariant = "menu" | "sheet";

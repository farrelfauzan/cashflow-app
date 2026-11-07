// What a failed linkMyTelegram shows: no bot here after all (the menu item goes), or a message in
// the dialog.
export type StaffTelegramErrorView =
  { kind: "hidden" } | { kind: "alert"; message: string };

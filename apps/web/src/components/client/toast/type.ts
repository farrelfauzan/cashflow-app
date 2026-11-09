// A toast's meaning, the same scale as the buttons (decision T-093, board "Notifikasi singkat"):
// create green, destructive red, caution orange, neutral ink (copy and other information).
export type ToastTone = "create" | "destructive" | "caution" | "neutral";

export type ToastMessage = {
  tone: ToastTone;
  // Short, names the thing: "Penyewa ditambahkan".
  title: string;
  // One line more, only what the screen already shows; null for none.
  description: string | null;
  // An icon for what happened, when the tone's own does not say it: "copy" for a copied link,
  // "send" for a message sent again. A neutral toast without one shows the information icon.
  icon?: ToastIcon;
};

export type ToastIcon = "copy" | "send";

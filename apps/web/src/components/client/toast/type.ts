// A toast's meaning, the same scale as the buttons (decision T-093, board "Notifikasi singkat"):
// create green, destructive red, caution orange, neutral ink (copy and other information).
export type ToastTone = "create" | "destructive" | "caution" | "neutral";

export type ToastMessage = {
  tone: ToastTone;
  // Short, names the thing: "Penyewa ditambahkan".
  title: string;
  // One line more, only what the screen already shows; null for none.
  description: string | null;
};

// Which step of the staff login is showing.
export type LoginStep = "phone" | "code";

// Where the code step says the latest code went. "telegram-or-whatsapp" only when the
// environment can send login codes over Telegram (a demo environment, decision 42) and the
// code was not asked for over WhatsApp only; the screen never says which channel was used.
export type LoginCodeChannel = "telegram-or-whatsapp" | "whatsapp";

// The code step's paragraph after "Jika nomor <phone>": `lead`, then `tail` kept on one line.
export type CodeSentEnding = { lead: string; tail: string };

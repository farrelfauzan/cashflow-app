import type { SeededMember } from "./staff";

// The seeded members login-telegram.spec.ts signs in through the UI, one per project and test. Like
// LOGIN_PHONES (staff.ts), each is used by one test in one project only and by no other spec,
// and none is signed in by the setup project (SETUP_STAFF), so the phone's limits (3 code
// requests and 10 verifies per 15 minutes) are its own. Both are active karyawan of Kos Bulanan
// Sejahtera only (seed-demo), so the picker goes straight in. The Telegram test spends 2 code
// requests, the production test 1.
export const LOGIN_TELEGRAM_PHONES: Record<"ipad" | "phone", SeededMember> = {
  ipad: { phone: "0812-0009-0016", name: "Mas Mahendra Putra" },
  phone: { phone: "0812-0009-0018", name: "Mas Panji Laksana" },
};

// The production screen (telegramAvailable false), emulated for the browser only.
export const LOGIN_PRODUCTION_PHONES: Record<"ipad" | "phone", SeededMember> = {
  ipad: { phone: "0812-0009-0019", name: "Mbak Rani Puspasari" },
  phone: { phone: "0812-0009-0020", name: "Mas Sigit Purnomo" },
};

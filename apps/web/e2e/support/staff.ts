import { join } from "node:path";

// Staff members created by `go run ./cmd/admin seed-demo` (services/api/internal/demoseed).
// Specs assert on these names, so they must match the seed exactly.

export type SeededStaff = {
  // As the seed and a person would type it; the devlog file uses E.164.
  phone: string;
  name: string;
  organizations: readonly string[];
  // The signed-in browser state the setup project saves. e2e/.auth/ is git-ignored and
  // Prettier-ignored.
  storageState: string;
};

// Admin in Griya Sejahtera and Supervisor in Kost Putri Anggrek: one login shows both
// organizations. Only one phone is signed in per run, shared by both projects, because the API
// allows 3 login code requests per phone per 15 minutes.
export const HENDRA: SeededStaff = {
  phone: "0812-0009-0001",
  name: "Pak Hendra Gunawan",
  organizations: ["Griya Sejahtera", "Kost Putri Anggrek"],
  storageState: join(__dirname, "..", ".auth", "hendra.json"),
};

// Admin limited to Kos Melati, in Griya Sejahtera only: one organization, so the picker goes
// straight in. Signed in by the setup project only.
export const SARI: SeededStaff = {
  phone: "0812-0001-0004",
  name: "Mbak Sari Wulandari",
  organizations: ["Griya Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "sari.json"),
};

// Karyawan in Kost Putri Anggrek only: sees only the tenants assigned to him. Signed in by the
// setup project only.
export const RUDI: SeededStaff = {
  phone: "0812-0002-0003",
  name: "Mas Rudi Hartanto",
  organizations: ["Kost Putri Anggrek"],
  storageState: join(__dirname, "..", ".auth", "rudi.json"),
};

// Every member the setup project signs in once per run.
export const SETUP_STAFF: readonly SeededStaff[] = [HENDRA, SARI, RUDI];

// A seeded member the login spec signs in through the UI. Each is used by one test in one
// project only, so parallel tests and reruns never share a phone's limits (3 code requests and
// 10 verifies per 15 minutes). None of them is signed in by the setup project (SETUP_STAFF).
export type SeededMember = Pick<SeededStaff, "phone" | "name">;

export type LoginPhones = {
  // Signs in from plain /login, then signs out.
  signIn: SeededMember;
  // Signs in from /login?next=...
  signInWithNext: SeededMember;
  // Types a wrong code, asks for a new one, then signs in with it (2 code requests).
  wrongCode: SeededMember;
  // Well-formed numbers that seed-demo never creates. The API answers them like any other.
  unregistered: string;
  // Requests codes until the API answers 429 (up to 4 requests).
  rateLimited: string;
};

export const LOGIN_PHONES: Record<"ipad" | "phone", LoginPhones> = {
  ipad: {
    signIn: { phone: "0812-0001-0001", name: "Bu Ani Rahmawati" },
    signInWithNext: { phone: "0812-0001-0002", name: "Pak Budi Hartono" },
    wrongCode: { phone: "0812-0001-0003", name: "Mas Dimas Saputra" },
    unregistered: "0812-9000-0101",
    rateLimited: "0812-9000-0201",
  },
  phone: {
    signIn: { phone: "0812-0002-0002", name: "Mbak Wulan Sari" },
    signInWithNext: { phone: "0812-0001-0005", name: "Mas Joko Prasetyo" },
    wrongCode: { phone: "0812-0002-0001", name: "Ibu Lestari Handayani" },
    unregistered: "0812-9000-0102",
    rateLimited: "0812-9000-0202",
  },
};

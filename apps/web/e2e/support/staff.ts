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

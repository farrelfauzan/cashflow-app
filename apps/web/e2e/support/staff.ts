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

// Kos Kunjungan Asri, the organization seed-demo makes only for the karyawan field specs: one
// property, Kos Asri Kaliurang (Rp800.000 a month), every tenant overdue once "admin run-day" has
// run for it. Its staff are members of it only (no picker), and each spec has its own karyawans,
// one per size project, so no two specs record on the same tenants.
export const VISIT_ORG = "Kos Kunjungan Asri";

// Admin in Griya Sejahtera and Supervisor in Kost Putri Anggrek: one login shows both
// organizations. Only one phone is signed in per run, shared by every project, because the API
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

// Admin of the whole of Kos Bulanan Sejahtera, the organization seeded with enough rows to span
// pages: 27 members, 25 properties, 30 units in Kos Sejahtera Utama and 45 tenants. Signed in by
// the setup project only.
export const DIMAS: SeededStaff = {
  phone: "0812-0009-0027",
  name: "Pak Dimas Prakoso",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "dimas.json"),
};

// Admin limited to Kontrakan Seturan in Kos Bulanan Sejahtera: the pages and totals count only
// the 8 tenants she may see. Signed in by the setup project only.
export const LESTARI: SeededStaff = {
  phone: "0812-0009-0028",
  name: "Mbak Lestari Anjani",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "lestari.json"),
};

// Karyawan of the whole of Kos Bulanan Sejahtera who never collects in any spec, so he holds no
// cash and has no deposit: the real empty state of "Setor tunai". Signed in by the setup project
// only.
export const UDIN: SeededStaff = {
  phone: "0812-0009-0022",
  name: "Mas Udin Saefudin",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "udin.json"),
};

// Karyawan of the whole of Kos Bulanan Sejahtera, collector of Oky Firmansyah (Kamar 15). Only the
// admin's "Setoran tunai" spec records and deposits her cash (on the iPad project), so no other
// spec races it. Signed in by the setup project only.
export const JUWITA: SeededStaff = {
  phone: "0812-0009-0013",
  name: "Mbak Juwita Sari",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "juwita.json"),
};

// Karyawan of the whole of Kos Bulanan Sejahtera, collector of Qori Amalia (Kamar 17), Rizki
// Ananda (Kamar 18) and Toko Berkah Jaya (Ruko 1). Only the "Antrean cek" spec records his cash,
// deposits it and records visit outcomes (on the iPad project), so no other spec races it.
// Signed in by the setup project only.
export const KURNIA: SeededStaff = {
  phone: "0812-0009-0014",
  name: "Mas Kurnia Aji",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "kurnia.json"),
};

// Karyawan of the whole of Kos Bulanan Sejahtera, one per size project, for "Terima tunai"
// (e2e/field-cash.spec.ts): the cash a karyawan holds is one total per member, so each project
// records cash as its own karyawan and checks that total exactly. No other spec uses them or the
// tenants they collect from. Signed in by the setup project only.
//
// iPad: collector of Indah Kusuma Wardani (Kamar 9), Joni Hermawan (Kamar 10) and Kurniawan Adi
// (Rumah 6).
export const GALIH: SeededStaff = {
  phone: "0812-0009-0010",
  name: "Mas Galih Pradana",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "galih.json"),
};

// Phone: collector of Sinta Maharani (Kamar 19), Tegar Wijaya (Kamar 20) and Warung Makan Bu Tini
// (Ruko 2).
export const LINA: SeededStaff = {
  phone: "0812-0009-0015",
  name: "Mbak Lina Rosita",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "lina.json"),
};

// "Ambil ulang" (e2e/field-receipt-retake.spec.ts): Kos Kunjungan Asri karyawans whose cash only
// that spec records, so the totals the Kos Bulanan Sejahtera field specs check exactly are never
// touched by it. Signed in by the setup project only.

// iPad: collector of Nico Pranata (Kamar 13) and Satria Nugraha (Kamar 14).
export const NURUL: SeededStaff = {
  phone: "0812-0010-0021",
  name: "Mbak Nurul Hidayah",
  organizations: [VISIT_ORG],
  storageState: join(__dirname, "..", ".auth", "nurul.json"),
};

// Phone: collector of Galuh Kartiko (Kamar 15) and Zaskia Ardiningsih (Kamar 16).
export const BAGUS: SeededStaff = {
  phone: "0812-0010-0022",
  name: "Mas Bagus Wicaksono",
  organizations: [VISIT_ORG],
  storageState: join(__dirname, "..", ".auth", "bagus.json"),
};

// Desktop: collector of Ilham Sasongko (Kamar 17) and Tiara Anindita (Kamar 18).
export const MIRA: SeededStaff = {
  phone: "0812-0010-0023",
  name: "Mbak Mira Widyastuti",
  organizations: [VISIT_ORG],
  storageState: join(__dirname, "..", ".auth", "mira.json"),
};

// Desktop: collector of Farhan Maulana (Kamar 6), Hadi Susanto (Kamar 8) and Irfan Hakim
// (Rumah 4). Farhan's transfer proofs belong to check-queue-proofs.spec.ts; field-cash.spec.ts
// takes cash only from the other two.
export const FITRI: SeededStaff = {
  phone: "0812-0009-0009",
  name: "Mbak Fitri Handayani",
  organizations: ["Kos Bulanan Sejahtera"],
  storageState: join(__dirname, "..", ".auth", "fitri.json"),
};

// Every member the setup project signs in once per run.
export const SETUP_STAFF: readonly SeededStaff[] = [
  HENDRA,
  SARI,
  RUDI,
  DIMAS,
  LESTARI,
  UDIN,
  JUWITA,
  KURNIA,
  NURUL,
  BAGUS,
  MIRA,
  GALIH,
  LINA,
  FITRI,
];

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

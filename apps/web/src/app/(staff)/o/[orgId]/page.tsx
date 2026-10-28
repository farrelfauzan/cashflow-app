import { PendingSection } from "@/components/server/shell/pending-section";

// The Supervisor's home (Beranda) comes with phase B (W10).
export default function OrgHomePage() {
  return (
    <PendingSection
      title="Beranda"
      description="Ringkasan tagihan dan pembayaran akan tampil di sini."
    />
  );
}

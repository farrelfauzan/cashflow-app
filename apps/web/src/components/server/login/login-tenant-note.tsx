// Tenants never log in; this sends one who lands here back to their WhatsApp link. A box at the
// bottom on phones, plain centered text under the card on iPad.
export function LoginTenantNote() {
  return (
    <p className="rounded-xl bg-muted px-4 py-3.5 text-sm leading-normal text-muted-foreground md:bg-transparent md:p-0 md:text-center md:text-base">
      Penyewa tidak perlu masuk. Untuk membayar, buka link dari pesan WhatsApp
      tagihan Anda.
    </p>
  );
}

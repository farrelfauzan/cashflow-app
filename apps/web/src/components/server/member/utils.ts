export const ROLES = [
  {
    name: "Supervisor",
    text: "Melihat semua data dan laporan. Menyetujui gaji dan pengeluaran besar. Tidak mengubah data lain.",
  },
  {
    name: "Admin",
    text: "Mengelola properti, unit, penyewa, dan sewa. Mencatat dan mengecek pembayaran, membuat koreksi, mencatat gaji dan pengeluaran. Menambah anggota sebagai Admin atau Karyawan. Tidak bisa menyetujui.",
  },
  {
    name: "Karyawan",
    text: "Melihat daftar kunjungan dan penyewa yang ditugaskan, mencatat uang tunai dan hasil kunjungan, melihat slip gaji sendiri.",
  },
] as const;

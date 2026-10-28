import { ROLES } from "./utils";

// "Arti tiap peran": what each built-in role may do, in plain words (board Members.dc.html).
// Static copy; what a member may really do comes from the API.
export function RoleGuide() {
  return (
    <div className="flex flex-col gap-[18px]">
      {ROLES.map((role) => (
        <section key={role.name} className="flex flex-col gap-1.5">
          <h3 className="text-lg font-bold">{role.name}</h3>
          <p className="text-base leading-[1.55] text-foreground/85">
            {role.text}
          </p>
        </section>
      ))}
      <section className="flex flex-col gap-2 rounded-xl bg-secondary/60 p-3.5">
        <h3 className="text-base font-bold">Aturan untuk semua</h3>
        <p className="text-[15px] leading-[1.55] text-foreground/85">
          Tidak ada yang bisa mengubah perannya sendiri. Yang mengecek gaji atau
          mencatat pengeluaran tidak bisa menyetujuinya.
        </p>
      </section>
    </div>
  );
}

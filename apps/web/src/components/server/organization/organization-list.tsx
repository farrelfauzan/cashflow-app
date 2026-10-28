import { getMe } from "@/lib/api/generated/auth/auth";

// Reads the signed-in user through the generated client on the server: the mutator forwards
// the cf_session cookie, and a 401 redirects to /login. It reads the request (cookies), so with
// Cache Components it must render inside a <Suspense> boundary.
export async function OrganizationList() {
  const me = await getMe();

  return (
    <div className="flex flex-col gap-3">
      <p className="text-base">
        Masuk sebagai <span className="font-semibold">{me.user.name}</span>
      </p>
      <ul className="flex flex-col gap-2" aria-label="Organisasi">
        {me.organizations.map((organization) => (
          <li
            key={organization.id}
            className="rounded-lg border px-4 py-3 text-base"
          >
            {organization.name}
          </li>
        ))}
      </ul>
    </div>
  );
}

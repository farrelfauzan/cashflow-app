import { LoginScreen } from "@/components/server/login/login-screen";

export default function LoginPage({ searchParams }: PageProps<"/login">) {
  return <LoginScreen searchParams={searchParams} />;
}

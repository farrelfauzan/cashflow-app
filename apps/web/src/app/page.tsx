import { redirect } from "next/navigation";

import { DEFAULT_POST_LOGIN_PATH } from "@/lib/auth/post-login-path";

// src/proxy.ts answers "/" before this page renders. Should it ever render, the organization
// list is behind the same session gate, so a visitor without a session still lands on login.
export default function HomePage() {
  redirect(DEFAULT_POST_LOGIN_PATH);
}

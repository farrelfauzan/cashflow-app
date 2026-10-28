import { ApiError } from "@/lib/api/errors";
import { logout } from "@/lib/api/generated/auth/auth";
import { clearQueryCache } from "@/lib/query/query-client";

import { LOGIN_PATH } from "./next-path";

// Browser only. Ends the session on the API (which also clears the HttpOnly cf_session cookie),
// forgets every cached answer, and loads the login page fresh. A 401 means the session was
// already gone, which is the outcome the user asked for. Any other error is thrown for the
// caller to show, and the user stays signed in.
export async function signOut(): Promise<void> {
  try {
    await logout();
  } catch (error) {
    if (!(error instanceof ApiError && error.status === 401)) throw error;
  }
  clearQueryCache();
  window.location.assign(LOGIN_PATH);
}

import { isLoginPath, isTenantPagePath, safeNextPath } from "./next-path";

// Where a staff member lands after login when there is no usable `next`.
export const DEFAULT_POST_LOGIN_PATH = "/organizations";

// The page to open after a successful login: the `?next=` value when it is a safe same-origin
// staff path, otherwise the organization list. A login path would loop and a tenant path
// (/t/<token>) is never a staff destination.
export function postLoginPath(next: string | null | undefined): string {
  const path = safeNextPath(next);
  if (path === null || isLoginPath(path) || isTenantPagePath(path)) {
    return DEFAULT_POST_LOGIN_PATH;
  }
  return path;
}

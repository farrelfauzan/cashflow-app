// The path of an organization page: orgPath(id) is its home, orgPath(id, "members") a section.
export function orgPath(orgId: string, segment = ""): string {
  const base = `/o/${encodeURIComponent(orgId)}`;
  return segment === "" ? base : `${base}/${segment}`;
}

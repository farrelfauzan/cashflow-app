import { expect, type Page } from "@playwright/test";

import type {
  ChargeType,
  ChargeTypeList,
} from "../../src/lib/api/generated/model";

// The metered charge types (listrik, air) every organization uses. seed-demo makes both for
// Griya Sejahtera; an organization without them gets them here, once. Charge types belong to the
// whole organization and cannot be removed, so the specs find them by name and never make a
// second one.

export type MeteredTypes = { listrik: ChargeType; air: ChargeType };

async function listMetered(page: Page, orgId: string): Promise<ChargeType[]> {
  const response = await page.request.get(`/api/orgs/${orgId}/charge-types`);
  expect(response.status(), await response.text()).toBe(200);
  const list = (await response.json()) as ChargeTypeList;
  return list.chargeTypes.filter((type) => type.metered);
}

function named(types: readonly ChargeType[], name: string) {
  return types.find((type) => type.name.toLowerCase() === name.toLowerCase());
}

async function findOrCreateMetered(
  page: Page,
  orgId: string,
  name: string,
): Promise<ChargeType> {
  const existing = named(await listMetered(page, orgId), name);
  if (existing !== undefined) return existing;
  const response = await page.request.post(`/api/orgs/${orgId}/charge-types`, {
    data: { name, metered: true },
  });
  if (response.status() === 201) return (await response.json()) as ChargeType;
  // Another test made it first.
  const raced = named(await listMetered(page, orgId), name);
  if (raced !== undefined) return raced;
  expect(response.status(), await response.text()).toBe(201);
  throw new Error("unreachable");
}

// "Listrik" and "Air" as the signed-in member (the page's cookies), made only when missing.
export async function findOrCreateMeteredTypes(
  page: Page,
  orgId: string,
): Promise<MeteredTypes> {
  return {
    listrik: await findOrCreateMetered(page, orgId, "Listrik"),
    air: await findOrCreateMetered(page, orgId, "Air"),
  };
}

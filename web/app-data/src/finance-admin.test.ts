// SPDX-License-Identifier: AGPL-3.0-only

// The administrators' finance as data (#3264, ADR-0067, ADR-0068): which price of a cost item applies on a date, from
// the date of each revision; what each write of a cost item and a save of what is published read again, so that no
// reader's view (the public totals, a holder's of `view_finance`, the funding meter of the instance) shows the figures
// of before; and what running a billing import now answers, the rate limit's refusal apart.

import { describe, expect, test } from "bun:test";
import { costItemActions, costPriceOn, runBillingImport, saveFinanceSettings, type CostItem } from "./finance-admin";
import { fakeData } from "./test/data";

type Data = ReturnType<typeof fakeData>["data"];

/** The reads of the totals and of the meter: the administrators', a holder's of `view_finance`, the public's. */
const totalsViews = [
  "/admin/cost_totals",
  "/admin/income_totals",
  "/finance/cost_totals",
  "/finance/income_totals",
  "/instance/cost_totals",
  "/instance/income_totals",
  "/instance",
] as const;

/** Puts a read of each of `paths` in the cache, and tells which of them a write marked stale. */
function seeded(data: Data, paths: readonly string[]) {
  for (const path of paths) data.queryClient.setQueryData(["get", path, {}], { seeded: path });
  return () => paths.filter((path) => data.queryClient.getQueryState(["get", path, {}])?.isInvalidated === true);
}

/** The cost item of `key` as the fake holds it. */
async function item(data: Data, id: string): Promise<CostItem> {
  const { data: body } = await data.api.GET("/admin/cost_items/{id}", { params: { path: { id } } });
  if (body === undefined) throw new Error(`test: no cost item ${id}`);
  return body;
}

test("a price applies from the date of its revision, the one before it up to the day before", async () => {
  const { fake, data } = fakeData({ viewer: "hana" });
  // The VPS: ¥3,300 from its first payment on 2025-01-15, then ¥3,630 from 2026-04-15.
  const vps = await item(data, fake.financeAdmin.costId("vps"));
  expect(costPriceOn(vps, "2025-01-15")?.price).toEqual({ amount: "3300", currency: "JPY" });
  expect(costPriceOn(vps, "2026-04-14")?.price).toEqual({ amount: "3300", currency: "JPY" });
  expect(costPriceOn(vps, "2026-04-15")?.price).toEqual({ amount: "3630", currency: "JPY" });
  expect(costPriceOn(vps, "2026-04-15")?.effective_on).toBe("2026-04-15");
  expect(costPriceOn(vps, "2030-01-01")?.price).toEqual({ amount: "3630", currency: "JPY" });
});

describe("a write of a cost item reads again every view of the totals and the funding meter", () => {
  const writes: readonly [string, (data: Data, id: string) => Promise<unknown>][] = [
    [
      "recording one",
      (data) =>
        costItemActions(data).create({
          schedule: "weeks",
          interval: 2,
          starts_on: "2026-01-05",
          name: "Object storage",
          provider: "Example Cloud",
          category: "storage",
          price: { amount: "12.50", currency: "USD" },
        }),
    ],
    ["changing its provider", (data, id) => costItemActions(data).relabel(id, { provider: "Sakura Internet" })],
    [
      "revising its price",
      (data, id) =>
        costItemActions(data).revise(id, { effective_on: "2026-12-01", price: { amount: "4000", currency: "JPY" } }),
    ],
    ["ending it", (data, id) => costItemActions(data).end(id, "2027-01-15")],
    ["deleting it", (data, id) => costItemActions(data).remove(id)],
  ];
  for (const [what, write] of writes)
    test(what, async () => {
      const { fake, data } = fakeData({ viewer: "hana" });
      const stale = seeded(data, totalsViews);
      await write(data, fake.financeAdmin.costId("vps"));
      expect(stale()).toEqual([...totalsViews]);
    });
});

test("saving what is published reads again the readers' totals and the instance, which holds the funding meter", async () => {
  const { data } = fakeData({ viewer: "hana" });
  const readers = totalsViews.filter((path) => !path.startsWith("/admin/"));
  const stale = seeded(data, readers);
  await saveFinanceSettings(data, { finance_visibility: "totals", income_visibility: "totals" });
  expect(stale()).toEqual(readers);
});

test("an import run now is queued, and one within the hour of the last is refused as too soon", async () => {
  expect(await runBillingImport(fakeData({ viewer: "hana" }).data, "aws")).toBe("queued");
  expect(await runBillingImport(fakeData({ viewer: "hana", financeAdmin: { importTooSoon: true } }).data, "aws")).toBe(
    "too-soon",
  );
});

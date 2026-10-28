// SPDX-License-Identifier: AGPL-3.0-only
// The data of the supporters screen stories (supporterList.stories.tsx, supporter.stories.tsx): a server with the
// program on and three plans, and a list of members with long and right-to-left names.
import { http, HttpResponse } from "msw";
import {
  accountFixture,
  instanceFixture,
  longNameAccountFixture,
  rtlNameAccountFixture,
  type Schemas,
} from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";
import { supporterPlanFixture } from "./handlers";

export const coffeeId = "0192f0c4-7a8e-7000-8000-0000000000c1";
export const loafId = "0192f0c4-7a8e-7000-8000-0000000000c2";
const bakeryId = "0192f0c4-7a8e-7000-8000-0000000000c3";

/** Three plans: a cheap one, one with a long name and description, and one priced in dollars. */
export const plans: Schemas["SupporterPlan"][] = [
  supporterPlanFixture(coffeeId, { name: "Coffee", rank: 1, description: "Keeps the server's lights on." }),
  supporterPlanFixture(loafId, {
    name: "A whole loaf of sourdough every month",
    rank: 2,
    description:
      "Pays for the backups and the storage of everyone's pictures, with a badge on your profile and our thanks on the supporter list.",
    monthly_price: { amount: "1500", currency: "JPY" },
    yearly_price: { amount: "16200", currency: "JPY" },
  }),
  supporterPlanFixture(bakeryId, {
    name: "Bakery",
    rank: 3,
    monthly_price: { amount: "25.00", currency: "USD" },
    yearly_price: { amount: "270.00", currency: "USD" },
  }),
];

/** GET /instance with the program on and the list published to `list`. */
export function programOn(list: Schemas["SupporterListVisibility"] = "public") {
  return http.get(`${apiBase}/instance`, () =>
    HttpResponse.json({
      ...instanceFixture,
      supporters_enabled: true,
      supporter_list_visibility: list,
      commerce_disclosure_url: "https://my.example/about/commerce",
    }),
  );
}

/** GET /instance/supporter_plans with the three plans. */
export const offeringPlans = http.get(`${apiBase}/instance/supporter_plans`, () => HttpResponse.json({ items: plans }));

/** The members of the supporter list, a few on each plan. */
export const members: Schemas["SupporterListEntry"][] = [
  { plan_id: bakeryId, account: longNameAccountFixture },
  { plan_id: loafId, account: { ...rtlNameAccountFixture, domain: instanceFixture.domain, acct: "abdulrahman" } },
  ...["Dana", "Eiji", "Fatima", "Gustav", "Hanna", "Ines"].map((name, i) => ({
    plan_id: i < 2 ? loafId : coffeeId,
    account: accountFixture(60 + i, name, instanceFixture.domain),
  })),
];

/** GET /instance/supporters listing `items` by the three plans. */
export function listing(items: Schemas["SupporterListEntry"][]) {
  return http.get(`${apiBase}/instance/supporters`, () =>
    HttpResponse.json({ plans: plans.map(({ id, name }) => ({ id, name })), items }),
  );
}

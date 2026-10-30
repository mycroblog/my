// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the landing feature (web-client/README.md §7): the instance's counts (GET
// /instance/overview), and the account to contact about it (GET /instance `contact`).
import { http, HttpResponse } from "msw";
import { accountFixture, instanceFixture, type Schemas } from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";

/** An administrator who shows the badge of the role: the account to contact about the instance. */
export const contactFixture: Schemas["Account"] = {
  ...accountFixture(7, "Olive", instanceFixture.domain),
  badges: [{ role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Admin", color: "red" }],
};

/** A small server: 120 members, a few public posts each day, started in March 2026. */
export const overviewFixture: Schemas["InstanceOverview"] = {
  counts: {
    members: 120,
    daily_posts: [3, 5, 2, 8, 6, 4, 7],
    started_at: "2026-03-14T00:00:00Z",
    counted_at: "2026-10-28T00:00:00Z",
  },
};

/** The instance with Olive as the account to contact about it (the setting `contact_account`). */
export const instanceWithContact: Schemas["Instance"] = { ...instanceFixture, contact: { account: contactFixture } };

export const handlers = [http.get(`${apiBase}/instance/overview`, () => HttpResponse.json(overviewFixture))];

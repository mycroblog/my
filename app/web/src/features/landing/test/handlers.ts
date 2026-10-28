// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the landing feature (web-client/README.md §7): the instance's counts and the people who
// run it (GET /instance/overview).
import { http, HttpResponse } from "msw";
import { accountFixture, instanceFixture, type Schemas } from "../../../test/fixtures";
import { apiBase } from "../../../test/msw/http";

/** An administrator who shows the badge of the role, the one operator of the default overview. */
export const operatorFixture: Schemas["Account"] = {
  ...accountFixture(7, "Olive", instanceFixture.domain),
  badges: [{ role_id: "0192f0c4-7a8e-7000-8000-0000000000b1", label: "Admin", color: "red" }],
};

/** A small server: 120 members, a few public posts each day, started in March 2026, run by Olive. */
export const overviewFixture: Schemas["InstanceOverview"] = {
  counts: {
    members: 120,
    daily_posts: [3, 5, 2, 8, 6, 4, 7],
    started_at: "2026-03-14T00:00:00Z",
    counted_at: "2026-10-28T00:00:00Z",
  },
  operators: [operatorFixture],
};

export const handlers = [http.get(`${apiBase}/instance/overview`, () => HttpResponse.json(overviewFixture))];

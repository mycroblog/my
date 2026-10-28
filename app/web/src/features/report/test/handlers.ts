// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the report feature (web-client/README.md §7): POST /reports files the report it is
// sent, against the accounts the fixtures know, and answers it as the server does. The rules that a
// violation cites are in `rulesFixture`, which a test serves in GET /instance with `instanceWithRules`.
import { http, HttpResponse } from "msw";
import { instanceFixture, otherAccountFixture, remoteAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** Two rules in effect, the second with a hint. */
export const rulesFixture: Schemas["InstanceRule"][] = [
  { id: "0192f0c4-7a8e-7000-8000-0000000007a1", text: "Be kind to each other" },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000007a2",
    text: "No advertising",
    hint: "Posts that sell something without being asked.",
  },
];

/** GET /instance with `rulesFixture` in effect. */
export const instanceWithRules = http.get(`${apiBase}/instance`, () =>
  HttpResponse.json({ ...instanceFixture, rules: rulesFixture }),
);

const reportable = new Map([otherAccountFixture, remoteAccountFixture].map((account) => [account.id, account]));

export const handlers = [
  http.post(`${apiBase}/reports`, async ({ request }) => {
    const body = (await request.clone().json()) as Schemas["CreateReportRequest"];
    return withViewer(request, () => {
      const account = reportable.get(body.account_id);
      if (account === undefined) return problem(422, "/api/v1/problems/validation", "Validation failed");
      const report: Schemas["Report"] = {
        id: "0192f0c4-7a8e-7000-8000-0000000007f1",
        category: body.category,
        comment: body.comment ?? "",
        forwarded: body.forward && account.acct.includes("@"),
        filed_at: "2026-10-17T00:00:00Z",
        target_account: account,
        status_ids: body.status_ids ?? [],
        rule_ids: body.rule_ids ?? [],
        state: "open",
      };
      return HttpResponse.json(report);
    });
  }),
];

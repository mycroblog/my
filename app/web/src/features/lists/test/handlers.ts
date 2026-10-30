// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the lists feature (web-client/README.md §7): the viewer has one list, "Friends", with
// Bob in it; every change succeeds. They need a token, as only the owner sees a list.
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas } from "../../../test/fixtures";
import { apiBase, problem, withViewer } from "../../../test/msw/http";

/** A list of the viewer; `overrides` replace its fields. */
export function listFixture(id: string, name: string, overrides: Partial<Schemas["List"]> = {}): Schemas["List"] {
  return { id, name, members_count: 0, created_at: "2026-10-20T09:00:00Z", ...overrides };
}

/** The viewer's list in the default handlers. */
export const friendsList = listFixture("0192f0c4-7a8e-7000-8000-00000000a001", "Friends", { members_count: 1 });

const notFound = () => problem(404, "/api/v1/problems/not-found", "Not Found");

export const handlers = [
  http.get(`${apiBase}/lists`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [friendsList] }))),
  http.post(`${apiBase}/lists`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(listFixture("0192f0c4-7a8e-7000-8000-00000000a0ff", "New list"))),
  ),
  http.get(`${apiBase}/lists/:id`, ({ request, params }) =>
    withViewer(request, () => (params["id"] === friendsList.id ? HttpResponse.json(friendsList) : notFound())),
  ),
  http.patch(`${apiBase}/lists/:id`, async ({ request, params }) => {
    const body = (await request.json()) as Schemas["ListRequest"];
    return withViewer(request, () =>
      params["id"] === friendsList.id ? HttpResponse.json({ ...friendsList, name: body.name }) : notFound(),
    );
  }),
  http.delete(`${apiBase}/lists/:id`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/lists/:id/accounts`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ accounts: [otherAccountFixture] } satisfies Schemas["AccountPage"])),
  ),
  http.post(`${apiBase}/lists/:id/accounts/:accountId`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.delete(`${apiBase}/lists/:id/accounts/:accountId`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
  http.get(`${apiBase}/accounts/:id/lists`, ({ request, params }) =>
    withViewer(request, () =>
      HttpResponse.json({ items: params["id"] === otherAccountFixture.id ? [friendsList] : [] }),
    ),
  ),
];

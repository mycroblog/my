// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the notifications feature (web-client/README.md §7): everything up to the follow is
// read, so the mention is unread.
import { http, HttpResponse } from "msw";
import { otherAccountFixture, type Schemas, statusFixture } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** Alice's notifications, newest first, for GET /notifications (operationId listNotifications). */
export const notificationsFixture: Schemas["Notification"][] = [
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000a2",
    type: "mention",
    created_at: "2026-10-16T09:00:00Z",
    account: otherAccountFixture,
    status: statusFixture("0192f0c4-7a8e-7000-8000-0000000000b1"),
  },
  {
    id: "0192f0c4-7a8e-7000-8000-0000000000a1",
    type: "follow",
    created_at: "2026-10-15T09:00:00Z",
    account: otherAccountFixture,
  },
];

export const handlers = [
  http.get(`${apiBase}/notifications`, ({ request }) =>
    withViewer(request, () => {
      const types = new URL(request.url).searchParams.getAll("types");
      const body: Schemas["NotificationPage"] = {
        items: notificationsFixture.filter((n) => types.length === 0 || types.includes(n.type)),
      };
      return HttpResponse.json(body);
    }),
  ),
  http.get(`${apiBase}/notifications/marker`, ({ request }) =>
    withViewer(request, () => {
      const body: Schemas["NotificationMarker"] = {
        last_read_id: "0192f0c4-7a8e-7000-8000-0000000000a1",
        unread_count: 1,
      };
      return HttpResponse.json(body);
    }),
  ),
  http.put(`${apiBase}/notifications/marker`, async ({ request }) => {
    const update = (await request.clone().json()) as Schemas["NotificationMarkerUpdate"];
    return withViewer(request, () => {
      const body: Schemas["NotificationMarker"] = { last_read_id: update.last_read_id, unread_count: 0 };
      return HttpResponse.json(body);
    });
  }),
  http.post(`${apiBase}/notifications/:id/dismiss`, ({ request }) =>
    withViewer(request, () => new HttpResponse(null, { status: 204 })),
  ),
];

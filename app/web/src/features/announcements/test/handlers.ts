// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the announcements feature (web-client/README.md §7): no announcements, and marking one
// read answers it read.
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../../test/fixtures";
import { apiBase, withViewer } from "../../../test/msw/http";

/** An announcement as GET /announcements answers it, published at 2026-10-21T00:00:00Z and unread, with `overrides`. */
export function announcementFixture(
  id: string,
  overrides: Partial<Schemas["Announcement"]> = {},
): Schemas["Announcement"] {
  return {
    id,
    title: "Maintenance on Sunday",
    content: "<p>Maintenance on Sunday.</p>",
    published_at: "2026-10-21T00:00:00Z",
    updated_at: "2026-10-21T00:00:00Z",
    read: false,
    ended: false,
    ...overrides,
  };
}

export const handlers = [
  http.get(`${apiBase}/announcements`, ({ request }) => withViewer(request, () => HttpResponse.json({ items: [] }))),
  http.post(`${apiBase}/announcements/:id/read`, ({ request, params }) =>
    withViewer(request, () => HttpResponse.json(announcementFixture(String(params["id"]), { read: true }))),
  ),
];

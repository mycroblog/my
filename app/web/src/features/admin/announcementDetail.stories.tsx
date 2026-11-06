// SPDX-License-Identifier: AGPL-3.0-only
// The page of one announcement (/admin/announcements/$id; web-client/screens/admin-announcements-detail.md) in the
// states of ADR-0079 §13: a published one within its window, a draft, a withdrawn one.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminAnnouncementFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

const id = "0199f0e0-0000-7000-8000-00000000a001";

/** The title and the text of a long announcement with a list and a link, and its window. */
const written = {
  title: "Scheduled maintenance on Sunday",
  text: "**Scheduled maintenance** on Sunday, November 2nd, from 02:00 to 04:00 UTC.\n\n- Posting and federation will pause for about thirty minutes while the database is upgraded to a new major version.\n- Notifications that arrive during the window are delivered afterwards.\n\nFollow the status page at https://status.my.example for updates.",
  content:
    '<p><strong>Scheduled maintenance</strong> on Sunday, November 2nd, from 02:00 to 04:00 UTC.</p><ul><li>Posting and federation will pause for about thirty minutes while the database is upgraded to a new major version.</li><li>Notifications that arrive during the window are delivered afterwards.</li></ul><p>Follow the status page at <a href="https://status.my.example">https://status.my.example</a> for updates.</p>',
  starts_at: "2026-10-28T00:00:00Z",
  ends_at: "2026-11-02T04:00:00Z",
  revised_at: "2026-10-27T09:00:00Z",
};

/** The announcement, published within its window. */
const maintenance = adminAnnouncementFixture(id, {
  ...written,
  state: "published",
  published_at: "2026-10-27T09:00:00Z",
});

function answering(announcement: Schemas["AdminAnnouncement"]) {
  return http.get(`${apiBase}/admin/announcements/:id`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(announcement)),
  );
}

const meta = {
  title: "Screens/Admin/Announcement",
  ...adminScreen(`/admin/announcements/${id}`, [answering(maintenance)]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A published announcement with a window: its state, the text as the users see it, the edit form and withdrawing. */
export const Normal: Story = {};

/** A draft, with Publish. */
export const Draft: Story = adminAnswering(answering(adminAnnouncementFixture(id, written)));

/** A withdrawn announcement: no edit form and nothing to withdraw. */
export const Withdrawn: Story = adminAnswering(answering({ ...maintenance, state: "withdrawn" }));

/** A moderator without manage_announcements. */
export const MissingPermission: Story = withoutPermission;

/** The announcement has not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/announcements/:id");

/** The server failed to answer the announcement. */
export const ServerError: Story = adminFailingOn("get", "/admin/announcements/:id");

/** In Japanese, with an announcement written in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    answering({
      ...maintenance,
      title: "定期メンテナンスのお知らせ",
      text: "**定期メンテナンス**のお知らせ：11月2日（日）2時から4時（UTC）まで、投稿と連合が一時的に止まります。",
      content:
        "<p><strong>定期メンテナンス</strong>のお知らせ：11月2日（日）2時から4時（UTC）まで、投稿と連合が一時的に止まります。</p>",
    }),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

// SPDX-License-Identifier: AGPL-3.0-only
// The server's announcements (/admin/announcements; web-client/screens/admin-announcements.md) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow } from "../../test/stories";
import { adminAnnouncementFixture } from "./test/handlers";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

/** A long announcement with a list and a link, published within a window. */
const maintenance = adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000a001", {
  text: "**Scheduled maintenance** on Sunday, November 2nd, from 02:00 to 04:00 UTC.\n\n- Posting and federation will pause for about thirty minutes while the database is upgraded to a new major version.\n- Notifications that arrive during the window are delivered afterwards.\n\nFollow the status page at https://status.my.example for updates.",
  content:
    '<p><strong>Scheduled maintenance</strong> on Sunday, November 2nd, from 02:00 to 04:00 UTC.</p><ul><li>Posting and federation will pause for about thirty minutes while the database is upgraded to a new major version.</li><li>Notifications that arrive during the window are delivered afterwards.</li></ul><p>Follow the status page at <a href="https://status.my.example">https://status.my.example</a> for updates.</p>',
  state: "published",
  starts_at: "2026-10-28T00:00:00Z",
  ends_at: "2026-11-02T04:00:00Z",
  published_at: "2026-10-27T09:00:00Z",
  updated_at: "2026-10-27T09:00:00Z",
});

/** The announcements of a server: published, a draft, and withdrawn ones. */
const announcements: Schemas["AdminAnnouncement"][] = [
  maintenance,
  adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000a002", {
    text: "Welcome to the **new** custom emoji picker!",
    content: "<p>Welcome to the <strong>new</strong> custom emoji picker!</p>",
  }),
  adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000a003", {
    text: "The server moves to a new home this weekend.",
    content: "<p>The server moves to a new home this weekend.</p>",
    state: "withdrawn",
    published_at: "2026-10-01T00:00:00Z",
    updated_at: "2026-10-03T00:00:00Z",
  }),
  adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000a004", {
    text: "Registrations are open again.",
    content: "<p>Registrations are open again.</p>",
    state: "published",
    published_at: "2026-09-15T00:00:00Z",
  }),
];

function listing(items: Schemas["AdminAnnouncement"][]) {
  return http.get(`${apiBase}/admin/announcements`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["AdminAnnouncementList"])),
  );
}

const meta = {
  title: "Screens/Admin/Announcements",
  ...adminScreen("/admin/announcements", [listing(announcements)]),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A published maintenance notice with a window, a draft, a withdrawn one and an older published one. */
export const Normal: Story = {};

/** A server without announcements. */
export const Empty: Story = adminAnswering();

/** A moderator without manage_announcements. */
export const MissingPermission: Story = withoutPermission;

/** The announcements have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/announcements");

/** The server failed to answer the announcements. */
export const ServerError: Story = adminFailingOn("get", "/admin/announcements");

/** In Japanese, with an announcement written in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...adminAnswering(
    listing([
      adminAnnouncementFixture("0199f0e0-0000-7000-8000-00000000a011", {
        text: "**定期メンテナンス**のお知らせ：11月2日（日）2時から4時（UTC）まで、投稿と連合が一時的に止まります。",
        content:
          "<p><strong>定期メンテナンス</strong>のお知らせ：11月2日（日）2時から4時（UTC）まで、投稿と連合が一時的に止まります。</p>",
        state: "published",
        published_at: "2026-10-27T09:00:00Z",
      }),
      ...announcements.slice(1),
    ]),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

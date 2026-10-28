// SPDX-License-Identifier: AGPL-3.0-only
// The edit history of a post (/@{acct}/status/{id}/history; FR-109) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { bob, picture, pictureHandlers, post } from "../../test/postStories";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";

/** Bob's post, edited three times. */
const edited = post(80, bob, {
  content: "<p>The bakery club meets on Saturday at 10:00 in the community hall, room 2. Bring an apron!</p>",
  media_attachments: [picture(30, "The community hall from the street")],
  edited_at: "2026-10-28T15:00:00Z",
  favourites_count: 7,
});

/** Its revisions, newest first: the time moved, a picture came, and a content warning went. */
const revisions: Schemas["StatusEdit"][] = [
  {
    content: edited.content,
    spoiler_text: "",
    sensitive: false,
    created_at: "2026-10-28T15:00:00Z",
    media_attachments: edited.media_attachments,
    emojis: [],
  },
  {
    content: "<p>The bakery club meets on Saturday at 10:00 in the community hall. Bring an apron!</p>",
    spoiler_text: "",
    sensitive: false,
    created_at: "2026-10-28T13:00:00Z",
    media_attachments: [],
    emojis: [],
  },
  {
    content: "<p>The bakery club meets on Saturday at 9:00 in the community hall.</p>",
    spoiler_text: "Club news",
    sensitive: false,
    created_at: "2026-10-28T12:00:00Z",
    media_attachments: [],
    emojis: [],
  },
];

/** GET /statuses/{id}/history answering `items`. */
function history(items: Schemas["StatusEdit"][]) {
  return http.get(`${apiBase}/statuses/:id/history`, () => HttpResponse.json({ items }));
}

const meta = {
  title: "Screens/Timeline/EditHistory",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/@${bob.acct}/status/${edited.id}/history`,
      signedIn: true,
      handlers: [
        ...pictureHandlers,
        http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(edited)),
        history(revisions),
      ],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Three revisions: the time moved, a picture came, and a content warning went. */
export const Normal: Story = {};

/** A post never edited: its one revision. */
export const NeverEdited: Story = answering(history(revisions.slice(-1)));

/** The post does not exist, or the viewer may not see it. */
export const NotFound: Story = answering(
  http.get(`${apiBase}/statuses/:id/history`, () => problem(404, "about:blank", "Not Found")),
);

/** The history has not arrived yet. */
export const Loading: Story = loadingOn("get", `/statuses/${edited.id}/history`);

/** The server failed to answer the history. */
export const ServerError: Story = failingOn("get", `/statuses/${edited.id}/history`);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

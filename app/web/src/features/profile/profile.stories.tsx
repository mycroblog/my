// SPDX-License-Identifier: AGPL-3.0-only
// The profile page (/@{acct}; web-client/screens/profile.md) in the states of ADR-0112 §1: another local account, the
// viewer's own, an account of another server, one that moved, and one that is not there (unknown or suspended).
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse, type RequestHandler } from "msw";
import { remoteAccountFixture, rtlNameAccountFixture, statusPage, viewerFixture } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import {
  answering,
  answeringWithErrorPage,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
  viewerWith,
} from "../../test/stories";
import { lookingUp, posting, postsOf, relatedAs, richBob } from "./test/storyFixtures";

/** Bob's profile and posts; a story's handlers replace the meta's, so a story that adds its own adds these after. */
const bob = [lookingUp(richBob), posting(postsOf(richBob))];

/** A story of the profile at `path`, whose server answers with `handlers` first. */
function at(path: string, ...handlers: RequestHandler[]) {
  return { parameters: { screen: { path, handlers } } };
}

const meta = {
  title: "Screens/Profile/Profile",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/@bob", signedIn: true, handlers: bob } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Another local account with profile text, fields, a badge, a pinned post and a few more. */
export const Normal: Story = {};

/** An account that follows the viewer, who has not followed back. */
export const FollowsYou: Story = answering(relatedAs({ followed_by: true }), ...bob);

/** An account whose new posts the viewer is notified of: the bell is on. */
export const Subscribed: Story = answering(relatedAs({ notify: true }), ...bob);

/** An account the viewer blocked. */
export const Blocked: Story = answering(relatedAs({ blocking: true }), ...bob);

/** Seen signed out: the page offers to sign in to follow. */
export const SignedOut: Story = { parameters: { screen: { signedIn: false } } };

/** The viewer's own profile, with the link to edit it. */
export const Own: Story = at(
  "/@alice",
  lookingUp({ ...viewerFixture, followers_count: 3, following_count: 5 }),
  posting(postsOf(viewerFixture)),
);

/**
 * An account of another server: its full handle and the note that only the posts this server received are shown. The
 * viewer does not follow it, so the bell cannot be pressed, and the line under the buttons says to follow first.
 */
export const Remote: Story = at(
  "/@carol@remote.example",
  lookingUp(remoteAccountFixture),
  posting(postsOf(remoteAccountFixture)),
);

/** An account of another server that the viewer follows, whose new posts the viewer is notified of. */
export const RemoteSubscribed: Story = at(
  "/@carol@remote.example",
  relatedAs({ following: true, notify: true }),
  lookingUp(remoteAccountFixture),
  posting(postsOf(remoteAccountFixture)),
);

/** An account that moved to another one. */
export const Moved: Story = answering(
  lookingUp({ ...richBob, moved: { ...remoteAccountFixture, acct: "bob@new.example", username: "bob" } }),
  ...bob,
);

/** The viewer's own account that moved, with the way to manage the move. */
export const OwnMoved: Story = at(
  "/@alice",
  viewerWith({ moved: remoteAccountFixture }),
  lookingUp({ ...viewerFixture, moved: remoteAccountFixture }),
  posting([]),
);

/** No account has the handle: an unknown one, or one suspended or deleted. */
export const NotFound: Story = answeringWithErrorPage(
  http.get(`${apiBase}/accounts/lookup`, () => problem(404, "about:blank", "Not Found")),
);

/** An account without posts. */
export const Empty: Story = answering(
  http.get(`${apiBase}/accounts/:id/statuses`, () => HttpResponse.json(statusPage([]))),
  lookingUp(richBob),
);

/** The account has not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/lookup");

/** The server failed to answer the account. */
export const ServerError: Story = failingOn("get", "/accounts/lookup");

/** In Japanese, with a Japanese profile. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    lookingUp({
      ...richBob,
      display_name: "パン屋のボブ",
      note: "<p>毎朝パンを焼いています。ライ麦、スペルト小麦、そしてときどき失敗作。</p>",
      fields: [{ name: "好きな粉", value: "近所の製粉所の石臼挽き全粒粉" }],
    }),
    ...bob,
  ),
};

/** Right to left, in the pseudo-locale ar-XB, with an Arabic name. */
export const RightToLeft: Story = {
  ...inLocale("ar-XB"),
  ...answering(
    lookingUp({ ...richBob, display_name: rtlNameAccountFixture.display_name, note: "<p>أخبز الخبز كل صباح.</p>" }),
    ...bob,
  ),
};

/** At 320 CSS px. */
export const Narrow: Story = narrow;

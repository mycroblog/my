// SPDX-License-Identifier: AGPL-3.0-only
// The posts that quote one of the viewer's posts (/@{acct}/status/{id}/quotes; FR-114) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { type Schemas, statusPage } from "../../test/fixtures";
import { apiBase, problem, withViewer } from "../../test/msw/http";
import { abdulrahman, bob, carol, haruka, maximiliana, pictureHandlers, post, viewer } from "../../test/postStories";
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

/** The viewer's post that the others quote. */
const quoted = post(70, viewer, {
  content: "<p>A small server is a neighbourhood: you know the people, and they know you.</p>",
  boosts_count: 12,
  favourites_count: 48,
});

/** The posts that quote it, newest first. */
const quotes = [
  post(71, bob, { content: "<p>This is why I stayed here.</p>" }),
  post(72, haruka, { content: "<p>小さなサーバーのよさがよく分かる一文。</p>", language: "ja" }),
  post(73, maximiliana, {
    content:
      "<p>Agreed, and I would add that a neighbourhood also needs someone who sweeps the street. Thank you to our moderators, who do it every day without anyone noticing.</p>",
    favourites_count: 9,
  }),
  post(74, abdulrahman, { content: "<p>كلام جميل وصحيح.</p>", language: "ar" }),
  post(75, carol, { content: "<p>Quoting this for my own server's newsletter.</p>", spoiler_text: "Meta" }),
].map((status) => ({ ...status, quote: { state: "accepted" as const, quoted_status: quoted } }));

/** GET /statuses/{id}/quotes answering `items`. */
function quoting(items: Schemas["Status"][]) {
  return http.get(`${apiBase}/statuses/:id/quotes`, ({ request }) =>
    withViewer(request, () => HttpResponse.json(statusPage(items))),
  );
}

const meta = {
  title: "Screens/Timeline/Quotes",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: `/@${viewer.acct}/status/${quoted.id}/quotes`,
      signedIn: true,
      handlers: [...pictureHandlers, quoting(quotes)],
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Five posts that quote the viewer's post, one behind a content warning. */
export const Normal: Story = {};

/** Nobody quoted the post yet. */
export const Empty: Story = answering(quoting([]));

/** Another account's post, which only its author may list the quotes of. */
export const NotFound: Story = answering(
  http.get(`${apiBase}/statuses/:id/quotes`, () => problem(404, "about:blank", "Not Found")),
);

/** The quotes have not arrived yet. */
export const Loading: Story = loadingOn("get", `/statuses/${quoted.id}/quotes`);

/** The server failed to answer the quotes. */
export const ServerError: Story = failingOn("get", `/statuses/${quoted.id}/quotes`);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

// SPDX-License-Identifier: AGPL-3.0-only
// The documents the server publishes (/about/terms, /about/privacy, /about/commerce) in
// the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import type { Schemas } from "../../test/fixtures";
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
} from "../../test/stories";

/** A document of several sections with a list, a table, a quote and a link, as the server renders the Markdown. */
const termsHtml = `<h2 id="accounts">Accounts</h2>
<p>You need to be at least 16 years old to sign up. One person may hold more than one account, as long as each says who runs it.</p>
<ul><li>Keep your password and passkeys to yourself.</li><li>Tell us at <a href="mailto:admin@my.example">admin@my.example</a> when someone else got into your account.</li></ul>
<h2 id="content">Content</h2>
<p>You keep the rights to what you post. You let this server store it, show it and send it to the servers of the people who follow you.</p>
<blockquote><p>Be kind. Disagree with ideas, not with people.</p></blockquote>
<h3 id="removal">Removal</h3>
<p>The moderators may remove a post that breaks the rules, and tell you which rule.</p>
<h2 id="costs">Costs</h2>
<table><thead><tr><th>Plan</th><th>Price</th></tr></thead><tbody><tr><td>Coffee</td><td>500 yen a month</td></tr><tr><td>A whole loaf</td><td>1,500 yen a month</td></tr></tbody></table>
<h2 id="changes">Changes to these terms</h2>
<p>We say so on the server a month before a change takes effect.</p>`;

/** The server answering every document with `html`. */
function publishing(html: string) {
  return http.get(`${apiBase}/instance/documents/:kind`, ({ params }) =>
    HttpResponse.json({
      kind: String(params["kind"]) as Schemas["InstanceDocumentKind"],
      html,
      revised_at: "2026-10-12T08:00:00Z",
    } satisfies Schemas["InstanceDocument"]),
  );
}

const meta = {
  title: "Screens/About/Documents",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/about/terms", handlers: [publishing(termsHtml)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The terms of service, with a table of contents, seen signed out. */
export const Terms: Story = {};

/** The privacy policy, seen signed in. */
export const Privacy: Story = { parameters: { screen: { path: "/about/privacy", signedIn: true } } };

/** The commerce disclosures. */
export const Commerce: Story = { parameters: { screen: { path: "/about/commerce" } } };

/** A document the server has not published. */
export const NotPublished: Story = answeringWithErrorPage(
  http.get(`${apiBase}/instance/documents/:kind`, () => problem(404, "about:blank", "Not found")),
);

/** The document has not arrived yet. */
export const Loading: Story = loadingOn("get", "/instance/documents/:kind");

/** The server failed to answer the document. */
export const ServerError: Story = failingOn("get", "/instance/documents/:kind");

/** In Japanese, with a document the administrators wrote in Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  ...answering(
    publishing(
      '<h2 id="accounts">アカウント</h2><p>16 歳以上の方が登録できます。</p><h2 id="content">投稿</h2><p>投稿の権利は投稿した人にあります。</p>',
    ),
  ),
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

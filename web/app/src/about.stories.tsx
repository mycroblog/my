// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page and its documents (ADR-0057 §5, ADR-0067 §8, ADR-0072 §2, ADR-0088 §20; FR-705, FR-718,
// FR-725, FR-810, FR-903, FR-905, FR-914), in the app's shell on the fake server (app.samples.tsx). The about page:
// every section the server can show (its banner, description, contact, rules, documents, moderated servers, external
// reviews, automatic deletion, crawlers, funding, support, and its facts), to a reader and to a visitor; the fake's own
// server, as it has always said of itself; a server that says almost nothing; a long summary; a visitor of a server
// that lists its moderated servers to signed-in people; and loading and failing. The documents: the terms, the privacy
// policy with a table, the commerce disclosure, one not published, loading and failing. The crawlers' page for each
// choice of search and AI.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

type Server = NonNullable<Fake["server"]>;

const meta = {
  title: "About",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/about" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The page at `at` of the fake's server with `server`, as `more` says (a visitor, a state of the server's answers). */
const at = (path: string, server: Server = {}, more: Omit<Fake, "at" | "server"> = {}): Pick<Story, "parameters"> => ({
  parameters: screen({ at: path, server, ...more }),
});

/** Everything the page can show: the program and the tips with the list, the costs and income with the meter. */
const everything: Server = {
  about: { says: "full", domains: "everyone" },
  thumbnail: true,
  supporters: { tips: {}, list: "everyone" },
  finance: { costs: "totals", income: "totals", meter: "amounts" },
  crawlers: { search: "allow", ai: "deny_training" },
};

/** Every section: the banner, description, contact, rules, documents, moderated servers, reviews, deletion, crawlers. */
export const Full: Story = at("/about", everything);
/** Every section a visitor is shown: the moderated servers too, which the server lists to everyone. */
export const FullVisitor: Story = at("/about", everything, { signedOut: true });
/** The fake's own server: its summary, rules, documents, crawlers, the month's meter, status page and source code. */
export const Default: Story = at("/about");
/** A server that says almost nothing: its name, domain, what it states to crawlers, its source code and version. */
export const Bare: Story = at("/about", {
  about: { says: "bare" },
  around: "none",
  crawlers: { search: "deny", ai: "deny" },
});
/** A summary and a description that run long, every section under them. */
export const Long: Story = at("/about", { ...everything, longSummary: true });
/** A visitor of a server that lists its moderated servers to signed-in people: the line with the way to sign in. */
export const DomainsSignedIn: Story = at("/about", { about: { domains: "signed_in" } }, { signedOut: true });
export const Loading: Story = at("/about", { about: { instance: "waits" } });
export const Failed: Story = at("/about", { about: { instance: "fails" } });

/** The terms of service: headings, paragraphs, a list, a link. */
export const Terms: Story = at("/about/terms");
/** The privacy policy, with a table and an email link, as a visitor reads it. */
export const Privacy: Story = at("/about/privacy", {}, { signedOut: true });
/** The commerce disclosure of the supporter plans: a table of terms and values. */
export const Commerce: Story = at("/about/commerce", { about: { says: "full" } });
/** A document the server has not published, with the way to the about page. */
export const Unpublished: Story = at("/about/terms", { about: { says: "bare" } });
export const DocumentLoading: Story = at("/about/terms", {}, { state: "loading" });
export const DocumentError: Story = at("/about/terms", {}, { state: "error" });

/** Search engines allowed, and nothing asked of AI. */
export const CrawlerAllow: Story = at("/about/crawler", { crawlers: { search: "allow", ai: "allow" } });
/** Search engines allowed, AI training refused. */
export const CrawlerNoTraining: Story = at("/about/crawler", { crawlers: { search: "allow", ai: "deny_training" } });
/** Neither, as a server with no setting of its own. */
export const CrawlerDeny: Story = at("/about/crawler", { crawlers: { search: "deny", ai: "deny" } });

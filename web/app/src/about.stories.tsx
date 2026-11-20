// SPDX-License-Identifier: AGPL-3.0-only

// The server's about page and its documents (ADR-0057 §5, ADR-0067 §8, ADR-0072 §2, ADR-0088 §20; FR-705, FR-718,
// FR-725, FR-810, FR-903, FR-905, FR-914), as their views draw what the fake server of @my/e2e answers, in a column of
// the timeline's width. The about page: every section the server can show (its banner, description, contact, rules,
// documents, moderated servers, external reviews, automatic deletion, crawlers, funding, support, and its facts); the
// fake's own server, as it has always said of itself; a server that says almost nothing; a long summary; a visitor of a
// server that lists its moderated servers to signed-in people; and loading and failing. The documents: the terms, the
// privacy policy with a table, the commerce disclosure, one not published, loading and failing. The crawlers' page for
// each choice of search and AI.

import { AboutView, CrawlerView, DocumentView, type AboutDomains, type DocumentRead } from "@my/app-features";
import { domainActionsShown, type Instance, type InstanceDocumentKind, type InstanceDomainAction } from "@my/app-data";
import { rootPx } from "@my/design-tokens";
import { createFakeApi, fakeOrigin, type FakeOptions } from "@my/e2e/fake";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import icon from "../../brand/icon.svg";

const rem = (px: number) => `${px / rootPx}rem`;

/** The main column, as wide as the timeline's at most, its edge's room on each side. */
const Column = ({ children }: { children: ReactNode }) => (
  <div style={{ maxWidth: rem(600), paddingInline: rem(16), paddingBlock: rem(16) }}>{children}</div>
);

/** What the fake answers at `path` of the API, as JSON, and its status. */
async function ask(options: FakeOptions, path: string): Promise<{ status: number; body: unknown }> {
  const response = await createFakeApi(options).serve(new Request(`${fakeOrigin}/api/v1${path}`));
  return { status: response.status, body: await response.json() };
}

/** What a story shows: its view and the fake's answers for it, read before it renders. */
type Shown =
  | { page: "about"; instance?: Instance; failed?: boolean; domains: AboutDomains }
  | { page: "document"; kind: InstanceDocumentKind; read: DocumentRead }
  | { page: "crawler"; instance: Instance };

const meta = {
  title: "About",
  tags: ["themes"],
  parameters: { layout: "fullscreen" },
  render: (_: unknown, { loaded }: { loaded: { shown?: Shown } }) => {
    const shown = loaded.shown;
    if (shown === undefined) return <Column>{null}</Column>;
    return (
      <Column>
        {shown.page === "about" ? (
          <AboutView
            instance={shown.instance}
            retry={shown.failed === true ? () => undefined : undefined}
            domains={shown.domains}
            builtInIcon={icon}
          />
        ) : shown.page === "document" ? (
          <DocumentView kind={shown.kind} read={shown.read} />
        ) : (
          <CrawlerView instance={shown.instance} />
        )}
      </Column>
    );
  },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The first page of the moderated domains as the view takes it, read to its end. */
const listOf = (items: InstanceDomainAction[]): AboutDomains => ({
  list: {
    items,
    status: "success",
    error: null,
    hasNextPage: false,
    isFetchingNextPage: false,
    failedNextPage: false,
    fetchNextPage: () => undefined,
    refetch: () => undefined,
  },
});

/** The about page of the fake's server with `options`, the reader signed in unless `visitor`. */
const about = (options: FakeOptions = {}): Pick<Story, "loaders"> => ({
  loaders: [
    async () => {
      const instance = (await ask(options, "/instance")).body as Instance;
      const domains: AboutDomains = domainActionsShown(instance, options.visitor !== true)
        ? listOf(((await ask(options, "/instance/domain_actions")).body as { items: InstanceDomainAction[] }).items)
        : instance.domain_actions_visibility === "signed_in"
          ? { signIn: "/login" }
          : undefined;
      return { shown: { page: "about", instance, domains } satisfies Shown };
    },
  ],
});

/** Everything the page can show: the program and the tips with the list, the costs and income with the meter. */
const everything: FakeOptions = {
  about: { says: "full", domains: "everyone" },
  thumbnail: true,
  supporters: { tips: {}, list: "everyone" },
  finance: { costs: "totals", income: "totals", meter: "amounts" },
  crawlers: { search: "allow", ai: "deny_training" },
};

/** Every section: the banner, description, contact, rules, documents, moderated servers, reviews, deletion, crawlers. */
export const Full: Story = about(everything);
/** The fake's own server: its summary, rules, documents, crawlers, the month's meter, status page and source code. */
export const Default: Story = about();
/** A server that says almost nothing: its name, domain, what it states to crawlers, its source code and version. */
export const Bare: Story = about({ about: { says: "bare" }, around: "none", crawlers: { search: "deny", ai: "deny" } });
/** A summary and a description that run long, every section under them. */
export const Long: Story = about({ ...everything, longSummary: true });
/** A visitor of a server that lists its moderated servers to signed-in people: the line with the way to sign in. */
export const DomainsSignedIn: Story = about({ about: { domains: "signed_in" }, visitor: true });
export const Loading: Story = { loaders: [() => ({ shown: { page: "about", domains: undefined } satisfies Shown })] };
export const Failed: Story = {
  loaders: [() => ({ shown: { page: "about", failed: true, domains: undefined } satisfies Shown })],
};

/** The document of `kind` as the fake's server with `options` answers it. */
const documentOf = (kind: InstanceDocumentKind, options: FakeOptions = {}): Pick<Story, "loaders"> => ({
  loaders: [
    async () => {
      const { status, body } = await ask(options, `/instance/documents/${kind}`);
      const read: DocumentRead =
        status === 404
          ? { status: "unpublished" }
          : { status: "success", document: body as Extract<DocumentRead, { status: "success" }>["document"] };
      return { shown: { page: "document", kind, read } satisfies Shown };
    },
  ],
});

/** The terms of service: headings, paragraphs, a list, a link. */
export const Terms: Story = documentOf("terms");
/** The privacy policy, with a table and an email link. */
export const Privacy: Story = documentOf("privacy_policy");
/** The commerce disclosure of the supporter plans: a table of terms and values. */
export const Commerce: Story = documentOf("commerce_disclosure", { about: { says: "full" } });
/** A document the server has not published, with the way to the about page. */
export const Unpublished: Story = documentOf("terms", { about: { says: "bare" } });
export const DocumentLoading: Story = {
  loaders: [() => ({ shown: { page: "document", kind: "terms", read: { status: "pending" } } satisfies Shown })],
};
export const DocumentError: Story = {
  loaders: [
    () => ({
      shown: { page: "document", kind: "terms", read: { status: "error", retry: () => undefined } } satisfies Shown,
    }),
  ],
};

/** The crawlers' page of the fake's server whose choices are `crawlers`. */
const crawler = (crawlers: NonNullable<FakeOptions["crawlers"]>): Pick<Story, "loaders"> => ({
  loaders: [
    async () => ({
      shown: { page: "crawler", instance: (await ask({ crawlers }, "/instance")).body as Instance } satisfies Shown,
    }),
  ],
});

/** Search engines allowed, and nothing asked of AI. */
export const CrawlerAllow: Story = crawler({ search: "allow", ai: "allow" });
/** Search engines allowed, AI training refused. */
export const CrawlerNoTraining: Story = crawler({ search: "allow", ai: "deny_training" });
/** Neither, as a server with no setting of its own. */
export const CrawlerDeny: Story = crawler({ search: "deny", ai: "deny" });

// SPDX-License-Identifier: AGPL-3.0-only

// A document of the server, for everyone, signed in or not (FR-705, ADR-0072 §13): its terms of service at
// /about/terms, its privacy policy at /about/privacy, the commerce disclosure of its supporter plans at /about/commerce.
// It reads as an article's page does: its title over the day it was last revised, then its text, as the server
// sanitized it, in whatever language its administrators wrote it. Where the server has not published it, the page says
// so with the way to the about page; else the screen leads it with that way (`AboutBack`). And the page for
// the operators of the websites that posts link to, at /about/crawler: what the crawler that fetches them for their
// cards does on this server (`link_cards`, `link_cards_ignore_robots_txt`), what it requests and sends and how often,
// how a website refuses it, and the contact of the server, or that it publishes none.

import { aboutPagePath, PostContent } from "@my/app-components";
import type { Instance, InstanceDocument, InstanceDocumentKind } from "@my/app-data";
import {
  ArticleParts,
  ArticleText,
  ArticleTitle,
  BackButton,
  dateFormat,
  FeedNotice,
  Stack,
  TextButton,
  TextLink,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import { Fragment, type ReactNode } from "react";
import { ContactDetails, useAboutStrings } from "./page";

/** The title of the page of a document, or of the crawlers' page. */
export function useAboutTitle(page: InstanceDocumentKind | "crawler"): string {
  const t = useAboutStrings();
  return page === "crawler" ? t("about.crawler") : t("about.document.title", { kind: page });
}

/** The way back to the about page, at the start of a document's page and of the crawlers' page. */
export function AboutBack({ onPress }: { onPress: () => void }): ReactNode {
  return <BackButton onPress={onPress}>{useAboutStrings()("about.back")}</BackButton>;
}

/** A document as the page reads it: until it is read, when it could not be, when the server has none, or it. */
export type DocumentRead =
  | { readonly status: "pending" }
  | { readonly status: "error"; readonly retry: () => void }
  | { readonly status: "unpublished" }
  | { readonly status: "success"; readonly document: InstanceDocument };

/** The page of the document of `kind`, as `read`. */
export function DocumentView({ kind, read }: { kind: InstanceDocumentKind; read: DocumentRead }): ReactNode {
  const t = useAboutStrings();
  const { locale } = useLocale();
  const zone = useTimeZone();
  const title = useAboutTitle(kind);
  if (read.status === "pending") return <FeedNotice lead>{t("about.document.loading")}</FeedNotice>;
  if (read.status === "error")
    return (
      <FeedNotice alert lead action={<TextButton onPress={read.retry}>{t("about.retry")}</TextButton>}>
        {t("about.document.failed")}
      </FeedNotice>
    );
  if (read.status === "unpublished")
    return (
      <FeedNotice lead action={<TextLink href={aboutPagePath}>{t("about.back")}</TextLink>}>
        {t("about.document.unpublished", { kind })}
      </FeedNotice>
    );
  const revised = dateFormat(locale, zone, { dateStyle: "long" }).format(new Date(read.document.revised_at));
  return (
    <ArticleParts>
      <ArticleTitle level={2} title={title} summary={t("about.document.revised", { date: revised })} />
      <ArticleText lang="">
        <PostContent html={read.document.html} lang="" article />
      </ArticleText>
    </ArticleParts>
  );
}

/** What a website's operator copies, the same in every language. */
const robotsExample = "User-agent: my-linkcard\nDisallow: /";
const directivesExample = `<meta name="robots" content="nosnippet">
<meta name="my-linkcard" content="noimageindex">
X-Robots-Tag: my-linkcard: nosnippet`;

/** What the crawlers' page reads of the instance. */
export type CrawlerInstance = Pick<
  Instance,
  "domain" | "version" | "link_cards" | "link_cards_ignore_robots_txt" | "contact"
>;

/**
 * The page for the operators of the websites that posts link to, on the crawler that fetches them for their cards: what
 * it does on this server by its settings, how to recognize it, what makes it fetch, what it fetches and sends, how
 * often, how to refuse it, and whom to ask.
 */
export function CrawlerView({ instance }: { instance: CrawlerInstance }): ReactNode {
  const t = useAboutStrings();
  const ignores = instance.link_cards_ignore_robots_txt;
  const state = !instance.link_cards
    ? t("about.crawler.state.off")
    : ignores
      ? t("about.crawler.state.ignores")
      : t("about.crawler.state.obeys");
  return (
    <ArticleParts>
      <ArticleTitle level={2} title={t("about.crawler")} summary="" />
      <ArticleText>
        <p>{t("about.crawler.intro")}</p>
        <blockquote>
          <p>{state}</p>
        </blockquote>
        <h2>{t("about.crawler.identity.title")}</h2>
        <p>{t("about.crawler.identity.user_agent")}</p>
        <pre>
          <code>{`my-linkcard/${instance.version} (+https://${instance.domain}/)`}</code>
        </pre>
        <p>{t("about.crawler.identity.token")}</p>
        <p>{t("about.crawler.identity.others")}</p>
        <h2>{t("about.crawler.trigger.title")}</h2>
        <p>{t("about.crawler.trigger.post")}</p>
        <p>{t("about.crawler.trigger.remote")}</p>
        <h2>{t("about.crawler.fetches.title")}</h2>
        <ul>
          <li>{t("about.crawler.fetches.page")}</li>
          <li>{t("about.crawler.fetches.image")}</li>
          {!ignores && <li>{t("about.crawler.fetches.robots")}</li>}
        </ul>
        <p>{t("about.crawler.fetches.nothing_else")}</p>
        <h2>{t("about.crawler.sends.title")}</h2>
        <p>{t("about.crawler.sends.headers")}</p>
        <p>{t("about.crawler.sends.not")}</p>
        <h2>{t("about.crawler.frequency.title")}</h2>
        <ul>
          <li>{t("about.crawler.frequency.once")}</li>
          <li>{t("about.crawler.frequency.failure")}</li>
          <li>{t("about.crawler.frequency.host")}</li>
          <li>{t("about.crawler.frequency.throttled")}</li>
          {!ignores && <li>{t("about.crawler.frequency.robots")}</li>}
        </ul>
        <h2>{t("about.crawler.opt_out.title")}</h2>
        <h3>{t("about.crawler.opt_out.robots.title")}</h3>
        {ignores && (
          <blockquote>
            <p>{t("about.crawler.opt_out.robots.ignored")}</p>
          </blockquote>
        )}
        <p>{t("about.crawler.opt_out.robots.intro")}</p>
        <pre>
          <code>{robotsExample}</code>
        </pre>
        <p>{t("about.crawler.opt_out.robots.star")}</p>
        {!ignores && <p>{t("about.crawler.opt_out.robots.timing")}</p>}
        <h3>{t("about.crawler.opt_out.directives.title")}</h3>
        <p>{t("about.crawler.opt_out.directives.intro")}</p>
        <div>
          <table aria-label={t("about.crawler.opt_out.directives.caption")}>
            <thead>
              <tr>
                <th scope="col">{t("about.crawler.opt_out.directives.directive")}</th>
                <th scope="col">{t("about.crawler.opt_out.directives.effect")}</th>
              </tr>
            </thead>
            <tbody>
              <Directive names={["nosnippet", "max-snippet:0"]}>
                {t("about.crawler.opt_out.directives.nosnippet")}
              </Directive>
              <Directive names={["noimageindex", "max-image-preview:none"]}>
                {t("about.crawler.opt_out.directives.noimageindex")}
              </Directive>
              <Directive names={["noindex", "none"]}>
                {ignores
                  ? t("about.crawler.opt_out.directives.noindex_ignored")
                  : t("about.crawler.opt_out.directives.noindex")}
              </Directive>
            </tbody>
          </table>
        </div>
        <p>{t("about.crawler.opt_out.directives.rest")}</p>
        <p>{t("about.crawler.opt_out.directives.where")}</p>
        <pre>
          <code>{directivesExample}</code>
        </pre>
        <h2>{t("about.crawler.contact.title")}</h2>
        <p>{t("about.crawler.contact.intro")}</p>
        {instance.contact === undefined && <p>{t("about.crawler.contact.none")}</p>}
      </ArticleText>
      {instance.contact !== undefined && (
        <Stack gap="content-paragraph" text="body">
          <ContactDetails contact={instance.contact} />
        </Stack>
      )}
    </ArticleParts>
  );
}

/** A row of the directives' table: the directives as a website's operator writes them, left to right, and the card. */
function Directive({ names, children }: { names: readonly string[]; children: string }) {
  return (
    <tr>
      <th scope="row">
        {names.map((name, i) => (
          <Fragment key={name}>
            {i > 0 && <br />}
            <code dir="ltr">{name}</code>
          </Fragment>
        ))}
      </th>
      <td>{children}</td>
    </tr>
  );
}

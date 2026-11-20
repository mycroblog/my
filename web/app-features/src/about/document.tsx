// SPDX-License-Identifier: AGPL-3.0-only

// A document of the server, for everyone, signed in or not (FR-705, ADR-0072 §13): its terms of service at
// /about/terms, its privacy policy at /about/privacy, the commerce disclosure of its supporter plans at /about/commerce.
// It reads as an article's page does: its title over the day it was last revised, then its text, as the server
// sanitized it, in whatever language its administrators wrote it. Where the server has not published it, the page says
// so with the way to the about page; else the screen leads it with that way (`AboutBack`). And what the server states
// to search engines and AI crawlers, at /about/crawler (ADR-0057 §5): its choice of each, in the words of its value,
// how the statement is made and how far it reaches, and that it is no legal advice.

import { aboutPagePath, PostContent } from "@my/app-components";
import type { Instance, InstanceDocument, InstanceDocumentKind } from "@my/app-data";
import {
  ArticleParts,
  ArticleText,
  ArticleTitle,
  BackButton,
  dateFormat,
  FeedNotice,
  TextButton,
  TextLink,
  useLocale,
  useTimeZone,
} from "@my/design-system";
import type { ReactNode } from "react";
import { useAboutStrings } from "./page";

/** The title of the page of a document, or of the crawlers' page. */
export function useAboutTitle(page: InstanceDocumentKind | "crawler"): string {
  const t = useAboutStrings();
  return page === "crawler" ? t("about.crawlers") : t("about.document.title", { kind: page });
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

/** The page of what `instance` states to search engines and AI crawlers. */
export function CrawlerView({ instance }: { instance: Pick<Instance, "search_indexing" | "ai_usage"> }): ReactNode {
  const t = useAboutStrings();
  return (
    <ArticleParts>
      <ArticleTitle level={2} title={t("about.crawlers")} summary="" />
      <ArticleText>
        <h2>{t("about.crawler.search")}</h2>
        <p>{t("about.crawler.search.text", { value: instance.search_indexing })}</p>
        <h2>{t("about.crawler.ai")}</h2>
        <p>{t("about.crawler.ai.text", { value: instance.ai_usage })}</p>
        <h2>{t("about.crawler.how")}</h2>
        <p>{t("about.crawler.how.text")}</p>
        <p>{t("about.crawler.legal")}</p>
      </ArticleText>
    </ArticleParts>
  );
}

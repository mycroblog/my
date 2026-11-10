// SPDX-License-Identifier: AGPL-3.0-only
import { ArticleCard, EmojiText } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { ArticleView, StatusView } from "../hooks/useStatusView";

/**
 * An article as a list shows it (ADR-0160 §1, §8): lists carry no body, so the card is its title, summary and cover,
 * one link to the article's page. A sensitive article shows its title and summary and conceals its cover (§6). An
 * article of another server reads the same as one of this server; one without a title says so in the app's words.
 */
export function StatusArticleCard({ article, view }: { article: ArticleView; view: StatusView }) {
  const t = useT();
  return (
    <ArticleCard
      href={view.path}
      title={article.title === null ? null : <EmojiText text={article.title} emojis={view.emojis} />}
      untitledLabel={t("client:status.article.untitled")}
      {...(article.summary !== null && { summary: <EmojiText text={article.summary} emojis={view.emojis} /> })}
      lang={view.lang}
      cover={article.cover}
      sensitive={view.sensitive}
      revealLabel={t("client:timeline.media.reveal")}
      kindLabel={t("client:status.article.kind")}
    />
  );
}

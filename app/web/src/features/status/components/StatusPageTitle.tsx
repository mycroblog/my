// SPDX-License-Identifier: AGPL-3.0-only
import { EmojiText } from "@my/ui";
import { useSuspenseQuery } from "@tanstack/react-query";
import { useApi } from "../../../lib/auth/SessionProvider";
import { useT } from "../../../lib/i18n";
import { statusQueryOptions } from "../api/status";

/**
 * The text of the heading of a post's page: "Post", or for an article its title, which is the page's level-one heading
 * (ADR-0160 §4: the article's own headings start at level two). The title is user content, in the article's language
 * and its own direction; an article of another server without one says so in the app's words. The route loader has
 * fetched the post.
 */
export function StatusPageTitle({ id }: { id: string }) {
  const t = useT();
  const api = useApi();
  const { data } = useSuspenseQuery(statusQueryOptions(api, id));
  const post = data.boost ?? data;
  const article = post.article;
  if (article === undefined) return t("client:status.title");
  if (article.title === "") return t("client:status.article.untitled");
  return (
    <span dir="auto" lang={post.language === "und" ? undefined : post.language}>
      <EmojiText text={article.title} emojis={post.emojis} />
    </span>
  );
}

// SPDX-License-Identifier: AGPL-3.0-only
import { ArticleCover, Heading, Hint, InlineError, PostContent, Section, Spinner, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { ArticleEditor } from "../hooks/useArticleEditor";

/**
 * The preview (web-client/screens/article-editor.md §3): the article as its page shows it (web-client §13), with the
 * title, the summary the server would give it, the cover, and the body the server rendered, which is the published
 * article's HTML. Under it, the size and images of the body against the instance's limits.
 */
export function ArticlePreview({ editor }: { editor: ArticleEditor }) {
  const t = useT();
  const { preview, fields, cover } = editor;
  const lang = fields.language === "und" ? undefined : fields.language;
  return (
    <Section title={t("client:articles.preview.heading")} surface="card">
      {preview.status === "empty" && <Hint>{t("client:articles.preview.empty")}</Hint>}
      {preview.status === "loading" && <Spinner label={t("client:shell.loading")} />}
      {preview.status === "error" && (
        <InlineError size="inherit">{t("client:articles.preview.failed", { reason: preview.message })}</InlineError>
      )}
      {preview.status === "ready" && (
        <Stack gap="4">
          {fields.title.trim() !== "" && (
            <Heading level={3} size="page" wrap="anywhere" dir="auto" lang={lang}>
              {fields.title.trim()}
            </Heading>
          )}
          {preview.summary !== "" && (
            <Stack gap="1">
              <Text as="p" size="lg" weight="medium" wrap="anywhere" dir="auto" lang={lang}>
                {preview.summary}
              </Text>
              {preview.summaryFromBody && <Hint>{t("client:articles.preview.summary_from_body")}</Hint>}
            </Stack>
          )}
          {fields.sensitive && <Hint>{t("client:articles.preview.sensitive")}</Hint>}
          {cover.status === "uploaded" && cover.image !== undefined && (
            <ArticleCover image={cover.image} lang={lang} rounded revealLabel={t("client:timeline.media.reveal")} />
          )}
          <PostContent html={preview.html} lang={lang} size="lg" article />
          <Text as="p" size="sm" tone={preview.isOverLimit ? "danger" : "muted"}>
            {t("client:articles.preview.size", {
              bytes: preview.bytes,
              maxBytes: preview.maxBytes,
              images: preview.images,
              maxImages: preview.maxImages,
            })}
          </Text>
          {preview.isOverLimit && <InlineError size="inherit">{t("client:articles.preview.over")}</InlineError>}
        </Stack>
      )}
    </Section>
  );
}

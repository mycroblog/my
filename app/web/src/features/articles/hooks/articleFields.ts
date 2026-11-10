// SPDX-License-Identifier: AGPL-3.0-only
// The article as the editor holds it (ADR-0160 §5), the requests made from it (a draft's save, an edit), and the
// fields of a draft or a published article read from the server. The editor keeps what the author is still writing:
// a draft keeps an empty title and a cover still processing; publishing checks the rest (`publishBlocker`).
import type { DraftRequest, ServerDraft, Visibility } from "../../compose";
import type { EditArticleRequest, StatusSource } from "../api/articles";
import { maxTags, parseTags, tagsText } from "./markdown";

/** The most characters of a title and of a summary (ArticleTitle, ArticleSummary). */
export const maxTitleLength = 200;
export const maxSummaryLength = 500;

/** The visibilities an article may have: never `direct` (ADR-0160 §5). */
export type ArticleVisibility = Exclude<Visibility, "direct">;

/** The cover image: one of the author's uploads with its alternative text. */
type CoverField = { mediaId: string; description: string };

/** What the editor holds of an article. */
export type ArticleFields = {
  title: string;
  summary: string;
  /** The body in Markdown. */
  text: string;
  /** The hashtags as typed (`parseTags` reads them). */
  tags: string;
  cover: CoverField | null;
  visibility: ArticleVisibility;
  /** A BCP 47 tag. */
  language: string;
  sensitive: boolean;
};

/** An empty article, with the author's default visibility and language. */
export function emptyFields(visibility: Visibility, language: string): ArticleFields {
  return {
    title: "",
    summary: "",
    text: "",
    tags: "",
    cover: null,
    visibility: articleVisibility(visibility),
    language,
    sensitive: false,
  };
}

/** The author's default visibility as an article's: a `direct` default becomes `private`, the nearest one. */
function articleVisibility(visibility: Visibility): ArticleVisibility {
  return visibility === "direct" ? "private" : visibility;
}

/** Whether the editor holds anything worth saving as a draft. */
export function hasContent(fields: ArticleFields): boolean {
  return (
    fields.title.trim() !== "" ||
    fields.summary.trim() !== "" ||
    fields.text.trim() !== "" ||
    fields.tags.trim() !== "" ||
    fields.cover !== null
  );
}

/** The cover as a request carries it, without an empty description. */
function coverInput(cover: CoverField | null) {
  if (cover === null) return {};
  const description = cover.description.trim();
  return { cover: { id: cover.mediaId, ...(description === "" ? {} : { description }) } };
}

/** The save of the editor's article as a draft (POST /drafts, PUT /drafts/{id}): the whole article, as it is. */
export function draftRequestOf(fields: ArticleFields): DraftRequest {
  return {
    text: fields.text,
    visibility: fields.visibility,
    language: fields.language,
    ...(fields.sensitive ? { sensitive: true } : {}),
    article: {
      title: fields.title.trim(),
      summary: fields.summary.trim(),
      tags: parseTags(fields.tags).slice(0, maxTags),
      ...coverInput(fields.cover),
    },
  };
}

/** The edit of a published article (PUT /articles/{id}); its visibility never changes. */
export function editRequestOf(fields: ArticleFields): EditArticleRequest {
  return {
    title: fields.title.trim(),
    summary: fields.summary.trim(),
    text: fields.text,
    tags: parseTags(fields.tags),
    language: fields.language,
    sensitive: fields.sensitive,
    ...coverInput(fields.cover),
  };
}

/** The editor's article of a draft read from the server; what the draft leaves out takes the author's defaults. */
export function fieldsOfDraft(
  draft: ServerDraft,
  defaults: { visibility: Visibility; language: string },
): ArticleFields {
  const article = draft.article;
  return {
    title: article?.title ?? "",
    summary: article?.summary ?? "",
    text: draft.text,
    tags: tagsText(article?.tags ?? []),
    cover:
      article?.cover === undefined ? null : { mediaId: article.cover.id, description: article.cover.description ?? "" },
    visibility: articleVisibility(draft.visibility),
    language: draft.language ?? defaults.language,
    sensitive: draft.sensitive,
  };
}

/** The editor's article of a published article: what its author wrote, and the post's visibility and sensitivity. */
export function fieldsOfSource(
  source: StatusSource,
  post: { visibility: Visibility; sensitive: boolean },
  defaultLanguage: string,
): ArticleFields {
  const article = source.article;
  return {
    title: article?.title ?? "",
    summary: article?.summary ?? "",
    text: source.text,
    tags: tagsText(article?.tags ?? []),
    cover:
      article?.cover === undefined ? null : { mediaId: article.cover.id, description: article.cover.description ?? "" },
    visibility: articleVisibility(post.visibility),
    language: source.language === "und" ? defaultLanguage : source.language,
    sensitive: post.sensitive,
  };
}

/** What keeps the article from being published, in the order the editor checks it. */
export type PublishBlocker = "title" | "text" | "tags" | "uploading";

/** The first reason the article cannot be published yet, or null. `uploading` while an image has no upload yet. */
export function publishBlocker(fields: ArticleFields, uploading: boolean): PublishBlocker | null {
  if (fields.title.trim() === "") return "title";
  if (fields.text.trim() === "") return "text";
  if (parseTags(fields.tags).length > maxTags) return "tags";
  if (uploading) return "uploading";
  return null;
}

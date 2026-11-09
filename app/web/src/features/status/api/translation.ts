// SPDX-License-Identifier: AGPL-3.0-only
// A post in the reader's language (FR-120, ADR-0148): the shape the card shows, whichever way it was translated, and
// the way through the server (POST /statuses/{id}/translation). A translation lives only in the Query cache, which is
// memory: nothing here reaches the browser's storage or the offline timeline (lib/storage), and the cache forgets it
// when the user changes (lib/auth/userData).
import { type PostContentNode, type SanitizedHtml, serverSanitizedHtml } from "@my/ui";
import type { components } from "@mycroblog/mykit";
import { type QueryClient, queryOptions, skipToken } from "@tanstack/react-query";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";

/** The problem type of a request over a monthly limit on translated characters. */
export const translationLimitType = "/api/v1/problems/translation-limit";
/** The problem type of a post that no provider may receive. */
export const translationUnavailableType = "/api/v1/problems/translation-unavailable";
/** The problem type of a language that no provider translates. */
export const translationUnsupportedType = "/api/v1/problems/translation-unsupported-language";
/** The problem type of a request that no provider answered usefully, which may pass. */
export const translationFailedType = "/api/v1/problems/translation-failed";

/**
 * The body of a translation: sanitized HTML from the server, or, from the browser, whose output no sanitizer read, the
 * structure of the post with the output as text (ADR-0148 §12).
 */
type TranslatedBody = { kind: "html"; html: SanitizedHtml } | { kind: "nodes"; nodes: readonly PostContentNode[] };

/**
 * Whose words a translation is, for the line that says so: a provider's machine translation, with what the provider
 * asks to be shown beside it, the browser's (its name, or null when it gives none that is known), or the author's own
 * text (the post has a body in the language, or no words to translate).
 */
export type TranslatedBy =
  | { kind: "provider"; name: string; href: string | null; logo: string | null }
  | { kind: "browser"; name: string | null }
  | { kind: "author" };

/** A post in another language, as the card shows it in place of the original. */
export type TranslatedPost = {
  /** The language of the translation (BCP 47). */
  language: string;
  /** The language it was translated from (BCP 47), which a provider may have detected; `und` when not known. */
  sourceLanguage: string;
  body: TranslatedBody;
  /** The content warning in plain text; empty when the post has none. */
  contentWarning: string;
  /** The alt text of the media that has any, by the media's ID. */
  altTexts: Readonly<Record<string, string>>;
  /** The titles of the poll's options, in their order; empty without a poll. */
  pollOptions: readonly string[];
  by: TranslatedBy;
};

/**
 * A way to translate a post into a language; the hook useTranslator chooses it. `onDownload` hears how much of a model
 * has come, from 0 to 1, when the way has one to download.
 */
export type Translator = (targetLanguage: string, onDownload: (loaded: number) => void) => Promise<TranslatedPost>;

/** Whose words the server says an answer is: the provider's that translated it, or, without one, the author's. */
function translatedBy(provider: components["schemas"]["TranslationProvider"] | undefined): TranslatedBy {
  return provider === undefined
    ? { kind: "author" }
    : { kind: "provider", name: provider.name, href: provider.url ?? null, logo: provider.logo_url ?? null };
}

/** What the card keeps of `answer`, the server's translation. */
function translatedPost(answer: components["schemas"]["StatusTranslation"]): TranslatedPost {
  const { provider } = answer;
  return {
    language: answer.language,
    sourceLanguage: answer.source_language,
    // `content` is sanitized HTML (api/openapi.yaml, StatusTranslation.content).
    body: { kind: "html", html: serverSanitizedHtml(answer.content) },
    contentWarning: answer.spoiler_text,
    altTexts: Object.fromEntries(answer.media_attachments.map(({ id, description }) => [id, description])),
    pollOptions: answer.poll_options,
    by: translatedBy(provider),
  };
}

/** Asks the server to translate post `id`; the providers its operator enabled do the work (ADR-0148 §8). */
export async function translateOnServer(api: Api, id: string, targetLanguage: string): Promise<TranslatedPost> {
  const { data, error } = await api.fetch.POST("/statuses/{id}/translation", {
    params: { path: { id } },
    body: { target_language: targetLanguage },
  });
  // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
  if (data === undefined) throw new ApiError(error);
  return translatedPost(data);
}

/**
 * Asks the server to translate post `id`, which report `report` points at, for a moderator (ADR-0148 §13): whatever
 * the post's visibility, but sent only where the operator lets the text of reports go.
 */
export async function translateReportedOnServer(
  api: Api,
  report: string,
  id: string,
  targetLanguage: string,
): Promise<TranslatedPost> {
  const { data, error } = await api.fetch.POST("/admin/reports/{id}/statuses/{status_id}/translation", {
    params: { path: { id: report, status_id: id } },
    body: { target_language: targetLanguage },
  });
  if (data === undefined) throw new ApiError(error);
  return translatedPost(data);
}

/** The comment of a report in another language, as the report's page shows it in place of the original. */
export type TranslatedComment = {
  /** The language of the translation (BCP 47). */
  language: string;
  /** The language it was translated from (BCP 47), which was detected; `und` when not known. */
  sourceLanguage: string;
  /** The comment in plain text. */
  text: string;
  by: TranslatedBy;
};

/** A way to translate the comment of a report into a language; `onDownload` as for a Translator. */
export type CommentTranslator = (
  targetLanguage: string,
  onDownload: (loaded: number) => void,
) => Promise<TranslatedComment>;

/** Asks the server to translate the comment of report `report` for a moderator; the server keeps no translation. */
export async function translateReportCommentOnServer(
  api: Api,
  report: string,
  targetLanguage: string,
): Promise<TranslatedComment> {
  const { data, error } = await api.fetch.POST("/admin/reports/{id}/comment/translation", {
    params: { path: { id: report } },
    body: { target_language: targetLanguage },
  });
  if (data === undefined) throw new ApiError(error);
  return {
    language: data.language,
    sourceLanguage: data.source_language,
    text: data.comment,
    by: translatedBy(data.provider),
  };
}

/**
 * The translation of one version of a post into a language, once it was asked for. The key has the version because a
 * translation belongs to it: after an edit the card has none and shows the original. Only the mutation of
 * useStatusTranslation writes it, since a translation costs characters and happens only when the reader asks (ADR-0148
 * §4). It is not an API path in the second place, so that invalidating the lists of a path never reaches it.
 */
export function statusTranslationQueryOptions(post: { id: string; edited_at?: string }, targetLanguage: string) {
  return queryOptions<TranslatedPost>({
    queryKey: ["status-translation", post.id, post.edited_at ?? null, targetLanguage],
    queryFn: skipToken,
    staleTime: Infinity,
  });
}

/** Keeps `translation` of one version of a post in a language (web-client/README.md S1). */
export function cacheStatusTranslation(
  queryClient: QueryClient,
  post: { id: string; edited_at?: string },
  targetLanguage: string,
  translation: TranslatedPost,
): void {
  queryClient.setQueryData(statusTranslationQueryOptions(post, targetLanguage).queryKey, translation);
}

/** Whether a BCP 47 tag has a script subtag: four letters after the language. */
function hasScript(tag: string): boolean {
  return /^[a-z]+-[a-z]{4}(-|$)/iu.test(tag);
}

/**
 * Whether a post in `language` already is in `target`, by the server's own rule for a body in the language (ADR-0148
 * §8): the same tag or, when neither has a script subtag, the same primary language, so that `zh-Hant` and `zh-Hans`
 * never count as one. A post in `und` is in no language yet, and the provider finds out which.
 */
export function isInLanguage(language: string, target: string): boolean {
  if (language.toLowerCase() === "und") return false;
  if (language.toLowerCase() === target.toLowerCase()) return true;
  if (hasScript(language) || hasScript(target)) return false;
  return language.split("-")[0]?.toLowerCase() === target.split("-")[0]?.toLowerCase();
}

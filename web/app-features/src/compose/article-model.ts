// SPDX-License-Identifier: AGPL-3.0-only

// What the article editor holds while the reader writes, and what it sends (FR-122, FR-123, ADR-0160): an article as
// the editor keeps it (its title, its Markdown body, its summary, its cover, its hashtags, its language, who may see it,
// reply to, boost and quote it, and whether it is sensitive), the limits it is checked by before it is sent (the server's preview counts the body's),
// and the bodies of a new article, of an edit and of a server draft made from it; and the article made from a server
// draft, or from what its author wrote of a published one, to go on writing it. No React and no server here: the hook
// (use-article.ts) holds it, the view shows it.

import {
  articleFieldLimits,
  type ArticleLimits,
  type ArticlePreview,
  type ArticleSource,
  type ComposeSettings,
  type CreateArticle,
  type DraftFailureReason,
  type DraftRequest,
  type EditArticle,
  type Interaction,
  type InteractionChoice,
  type MediaAttachment,
  type SavedDraft,
  type ServerDraft,
  type Status,
  type Visibility,
} from "@my/app-data";
import type { FieldMembers } from "../field-errors";
import { keptPolicy, policyChange } from "./model";

/** What the editor is open for: a new article, or an edit of the reader's published article of `id`. */
type ArticleTarget = { kind: "new" } | { kind: "edit"; id: string };

/** Who may see an article: any visibility but `direct` (ADR-0160 §5). */
export type ArticleVisibility = Exclude<Visibility, "direct">;

/** The visibilities from the widest, as the editor's menu lists them. */
export const articleVisibilities: readonly ArticleVisibility[] = ["public", "local", "unlisted", "private"];

/** The cover: its file while it uploads, the server's ID once it holds it, and its description. */
export type ArticleCover = {
  /** The file chosen, to upload again after a failure. */
  readonly file?: File | undefined;
  /** What the editor shows of it: the file's own preview while it uploads, the server's once ready. */
  readonly preview?: string | undefined;
  /** The server's ID of the file, once it holds it. */
  readonly id?: string | undefined;
  readonly state: "uploading" | "processing" | "failed" | "ready";
  readonly description: string;
};

/** An article as the editor holds it. */
export type ArticleDraft = {
  readonly target: ArticleTarget;
  readonly title: string;
  /** The body, in Markdown. */
  readonly text: string;
  /** The summary; empty makes one from the body. */
  readonly summary: string;
  readonly cover?: ArticleCover | undefined;
  /** The hashtags, without `#`, in order. */
  readonly tags: readonly string[];
  /** A BCP 47 tag; empty for the reader's default. */
  readonly language: string;
  readonly visibility: ArticleVisibility;
  /** Who may reply, boost and quote: an audience at once, and whether others may ask for approval. */
  readonly policy: Readonly<Record<Interaction, InteractionChoice>>;
  readonly sensitive: boolean;
  /** The draft on the server it was saved as, to save over or publish. */
  readonly saved?: SavedDraft | undefined;
  /** It was opened from the reader's drafts on the server (ADR-0156 §9). */
  readonly stored?: boolean | undefined;
  /** When it is to be published (ISO 8601); undefined to publish now. */
  readonly scheduledAt?: string | undefined;
  /** Its last schedule failed: the time it was for and why it was not published (ADR-0156 §8). */
  readonly failure?: { readonly publishAt: string; readonly reason: DraftFailureReason } | undefined;
  /** The Idempotency-Key of its publication, the same for every attempt. */
  readonly key: string;
};

/** An article's visibility from a post's: `direct`, which an article never is, is followers-only. */
const articleVisibility = (v: Visibility): ArticleVisibility => (v === "direct" ? "private" : v);

/** The language of a draft as the editor holds it: empty for none. */
const languageOf = (tag: string | undefined) => (tag === undefined || tag === "und" ? "" : tag);

/** A new article, empty, from the reader's settings. */
export function emptyArticle(settings: ComposeSettings, key: string): ArticleDraft {
  return {
    target: { kind: "new" },
    title: "",
    text: "",
    summary: "",
    tags: [],
    language: languageOf(settings.language),
    visibility: articleVisibility(settings.visibility),
    policy: settings.policy,
    sensitive: false,
    key,
  };
}

/** The cover as the server holds it: its file (undefined when it could not be read) and its description. */
const coverOf = (id: string, description: string | undefined, media: MediaAttachment | undefined): ArticleCover => ({
  id,
  preview: media?.preview_url ?? media?.url,
  // A file not read is still the server's: kept by its ID.
  state: media?.state === "failed" ? "failed" : "ready",
  description: description ?? "",
});

/**
 * The article of a draft the server keeps, to go on writing it (ADR-0156, ADR-0160 §2): its cover as the server holds
 * it now (`cover`, undefined when gone or never read), who may interact with it (the reader's defaults for the kinds it
 * leaves out), its schedule, and why its last schedule failed, while it has none.
 */
export function articleFromServer(
  server: ServerDraft,
  saved: SavedDraft,
  cover: MediaAttachment | undefined,
  settings: ComposeSettings,
  key: string,
): ArticleDraft {
  const a = server.article;
  const failed = server.scheduled_at === undefined ? server.last_schedule : undefined;
  return {
    target: { kind: "new" },
    title: a?.title ?? "",
    text: server.text,
    summary: a?.summary ?? "",
    cover: a?.cover === undefined ? undefined : coverOf(a.cover.id, a.cover.description, cover),
    tags: a?.tags ?? [],
    language: languageOf(server.language),
    visibility: articleVisibility(server.visibility),
    policy: keptPolicy(server.interaction_policy, settings),
    sensitive: server.sensitive,
    saved,
    stored: true,
    scheduledAt: server.scheduled_at,
    failure:
      failed?.outcome === "failed" ? { publishAt: failed.publish_at, reason: failed.reason ?? "invalid" } : undefined,
    key,
  };
}

/**
 * The reader's published article of `status`, from what its author wrote of it (`source`), to edit it; who may
 * interact with it, which an edit does not change, as it is.
 */
export function articleFromSource(
  status: Status,
  source: ArticleSource,
  settings: ComposeSettings,
  key: string,
): ArticleDraft {
  const a = source.article;
  const cover = status.article?.cover;
  return {
    target: { kind: "edit", id: status.id },
    title: a?.title ?? "",
    text: source.text,
    summary: a?.summary ?? "",
    cover:
      a?.cover === undefined
        ? undefined
        : {
            id: a.cover.id,
            preview: cover?.preview_url ?? cover?.url,
            state: "ready",
            description: a.cover.description ?? "",
          },
    tags: a?.tags ?? [],
    language: languageOf(source.language),
    visibility: articleVisibility(status.visibility),
    policy: keptPolicy(status.interaction_policy, settings),
    sensitive: status.sensitive,
    key,
  };
}

/** Whether anything is written that leaving would lose. */
export const writtenArticle = (d: ArticleDraft) =>
  d.title.trim() !== "" ||
  d.text.trim() !== "" ||
  d.summary.trim() !== "" ||
  d.cover !== undefined ||
  d.tags.length > 0;

/** The characters of a text, as the server counts them (code points). */
export const charsOf = (text: string) => Array.from(text).length;

/** What keeps an article from being published. */
export type ArticleProblems = {
  /** The title is blank. */
  readonly titleMissing: boolean;
  /** How many characters the title is over its limit. */
  readonly titleOver: number;
  /** How many characters the summary is over its limit. */
  readonly summaryOver: number;
  /** The body is blank. */
  readonly bodyMissing: boolean;
  /** The body's HTML is over its bytes, or it has more images than allowed, as the latest preview counts them. */
  readonly bytesOver: boolean;
  readonly imagesOver: boolean;
  /** The cover is still uploading, or failed. */
  readonly coverWaiting: boolean;
  readonly coverFailed: boolean;
};

/** What keeps `d` from being published, with the latest preview's count of its body (none yet: nothing over). */
export function articleProblems(d: ArticleDraft, preview: ArticlePreview | undefined): ArticleProblems {
  return {
    titleMissing: d.title.trim() === "",
    titleOver: Math.max(0, charsOf(d.title) - articleFieldLimits.title),
    summaryOver: Math.max(0, charsOf(d.summary) - articleFieldLimits.summary),
    bodyMissing: d.text.trim() === "",
    bytesOver: preview !== undefined && preview.bytes > preview.max_bytes,
    imagesOver: preview !== undefined && preview.images > preview.max_images,
    coverWaiting: d.cover !== undefined && (d.cover.state === "uploading" || d.cover.state === "processing"),
    coverFailed: d.cover?.state === "failed",
  };
}

/** Whether an article with `p` may be published. */
export const publishable = (p: ArticleProblems) =>
  !p.titleMissing &&
  p.titleOver === 0 &&
  p.summaryOver === 0 &&
  !p.bodyMissing &&
  !p.bytesOver &&
  !p.imagesOver &&
  !p.coverWaiting &&
  !p.coverFailed;

/** Whether the limits of a body are near (nine tenths used), or past. */
export function limitState(used: number, max: number): "ok" | "near" | "over" {
  if (used > max) return "over";
  return used >= max * 0.9 ? "near" : "ok";
}

/**
 * How much of the body's length limit is used, as a ratio to show as a whole percentage: past the limit never reads
 * 100% (at least 101%), and under it never rounds up to 100% (at most 99%), so that the figure says which side it is.
 */
export function lengthRatio(used: number, max: number): number {
  const ratio = used / max;
  if (used > max) return Math.max(ratio, 1.01);
  return used < max ? Math.min(ratio, 0.99) : 1;
}

/** The body's limits as the preview counts them, the instance's limits with nothing used until it answered. */
export const usage = (preview: ArticlePreview | undefined, limits: ArticleLimits) => ({
  bytes: preview?.bytes ?? 0,
  maxBytes: preview?.max_bytes ?? limits.max_bytes,
  images: preview?.images ?? 0,
  maxImages: preview?.max_images ?? limits.max_images,
});

/** The cover as the API takes it, once the server holds its file. */
const coverInput = (d: ArticleDraft) =>
  d.cover?.id === undefined ? {} : { cover: { id: d.cover.id, description: d.cover.description.trim() } };

/** The language as the API takes it: none for the reader's default. */
const languageInput = (d: ArticleDraft, settings?: ComposeSettings) => {
  const language = d.language !== "" ? d.language : settings?.language;
  return language === undefined || language === "" || language === "und" ? {} : { language };
};

/** An input of the article editor a refusal can stand beside. */
export type ArticleInput = "title" | "text" | "summary" | "cover" | "tags";

/**
 * Which input answers to which member of an article's body: a new one, an edit, or a draft (its body `text`, the rest
 * under `article`). The body answers for its length, its images and their descriptions; the hashtags for their count
 * and each one. `language` answers to no input: it stays the form's error.
 */
export const articleMembers: FieldMembers<ArticleInput> = {
  title: "title",
  text: "text",
  summary: "summary",
  tags: "tags",
  "tags[]": "tags",
  "cover.description": "cover",
  "article.title": "title",
  "article.summary": "summary",
  "article.tags": "tags",
  "article.tags[]": "tags",
  "article.cover.description": "cover",
};

/** Who may reply, boost and quote, as the API takes it: the kinds the reader changed from their defaults. */
const policyInput = (d: ArticleDraft, settings: ComposeSettings) => {
  const policy = policyChange(d.policy, settings);
  return policy === undefined ? {} : { interaction_policy: policy };
};

/** A new article, as the API takes it. */
export function toCreateArticle(d: ArticleDraft, settings: ComposeSettings): CreateArticle {
  return {
    title: d.title.trim(),
    text: d.text,
    visibility: d.visibility,
    sensitive: d.sensitive,
    ...policyInput(d, settings),
    ...(d.summary.trim() === "" ? {} : { summary: d.summary.trim() }),
    ...(d.tags.length === 0 ? {} : { tags: [...d.tags] }),
    ...coverInput(d),
    ...languageInput(d, settings),
  };
}

/** The new content of a published article, as the API takes it: its visibility never changes. */
export function toEditArticle(d: ArticleDraft): EditArticle {
  return {
    title: d.title.trim(),
    text: d.text,
    sensitive: d.sensitive,
    summary: d.summary.trim(),
    tags: [...d.tags],
    ...coverInput(d),
    ...languageInput(d),
  };
}

/** The first `n` characters of a text. */
const clip = (text: string, n: number) => Array.from(text).slice(0, n).join("");

/**
 * The whole content of the article's server draft, as the API takes it: a draft keeps an empty title and body, but its
 * title, summary and hashtags only within their limits, which a draft refuses past.
 */
export function toArticleDraft(d: ArticleDraft, settings: ComposeSettings): DraftRequest {
  return {
    text: d.text,
    visibility: d.visibility,
    sensitive: d.sensitive,
    ...policyInput(d, settings),
    ...languageInput(d, settings),
    article: {
      title: clip(d.title, articleFieldLimits.title),
      summary: clip(d.summary, articleFieldLimits.summary),
      tags: d.tags.slice(0, articleFieldLimits.tags),
      ...coverInput(d),
    },
  };
}

/** The Markdown of an image of the reader's upload in the body (ADR-0160 §4): the server shows it as an image. */
export const imageMarkdown = (id: string, alt: string) => `![${alt.replace(/[[\]]/g, "")}](media:${id})`;

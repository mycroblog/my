// SPDX-License-Identifier: AGPL-3.0-only
// What the post composer reads and writes (ADR-0028 §6, §7, ADR-0156): the settings a new post starts from, the post
// replied to or quoted, the media uploads, the drafts kept on the server (drafts.ts) and their schedules, and the post
// itself (or a draft published), or the edit of one. A posted status goes
// into the cache at once: at the top of the home timeline, and one more reply on the post it replies to, shown in its
// cached threads. The write is a plain request with the composer's Idempotency-Key (no outbox yet, #2892): a failure
// rejects with mykit's ProblemError or NetworkError, and the composer keeps what was written to send again with the
// same key.
import { ProblemError, uploadMedia, type components, type MediaAttachment } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { prependHomeStatuses } from "./arrivals";
import { cacheConversationPost } from "./conversations";
import {
  defaultDraftLimits,
  deleteDraft,
  DraftConflictError,
  DraftGoneError,
  invalidateDrafts,
  readDraft,
  savedOf,
  scheduleDraft,
  unscheduleDraft,
  type SavedDraft,
  type ServerDraft,
} from "./drafts";
import { editStatus, type EditStatus } from "./own-posts";
import { useAppData } from "./provider";
import { cachedStatus, shadowStatus, useShadow } from "./shadow";
import { findCachedCopy, type Status } from "./statuses";
import { cacheReply } from "./thread";

export type { MediaAttachment };

/** A new status, as the API takes it. */
export type CreateStatus = components["schemas"]["CreateStatusRequest"];

/** The content of a draft, as the API takes it. */
export type DraftRequest = components["schemas"]["DraftRequest"];

/** Who may see a status. */
export type Visibility = components["schemas"]["Visibility"];

/** Whom an interaction admits, narrowest last (ADR-0155 §2). */
export type Audience = components["schemas"]["InteractionAudience"];

/** The kinds of interaction a policy limits. */
export type Interaction = "reply" | "announce" | "quote";

/** Who may interact with a status in one way, as the API gives and takes it (ADR-0155 §2). */
export type InteractionRule = components["schemas"]["InteractionRule"];

/**
 * Who may interact in one way, as the composer offers it: an audience admitted at once, and whether anyone else may ask
 * for the author's approval (the API's `manual` of `everyone`); asking means nothing while the audience is everyone.
 */
export type InteractionChoice = { readonly audience: Audience; readonly approval: boolean };

/** A rule as the composer offers it: any `manual` reads as anyone else asking. */
export const choiceOf = (rule: InteractionRule): InteractionChoice => ({
  audience: rule.automatic,
  approval: rule.manual !== undefined,
});

/** The rule of a choice: its audience at once, and everyone else by approval when they may ask. */
export const ruleOf = (choice: InteractionChoice): InteractionRule =>
  choice.approval && choice.audience !== "everyone"
    ? { automatic: choice.audience, manual: "everyone" }
    : { automatic: choice.audience };

/** What a new post starts from, and what the server allows of it. */
export type ComposeSettings = {
  /** The most characters of the text and the content warning together. */
  readonly maxChars: number;
  /** Whether every image needs a description before it is posted. */
  readonly requireDescriptions: boolean;
  readonly visibility: Visibility;
  /** Whom each interaction admits, at once or by approval, by default. */
  readonly policy: Readonly<Record<Interaction, InteractionChoice>>;
  /** The default language of a post, a BCP 47 tag; `und` when there is none. */
  readonly language: string;
  /** The instance's domain, which a local post stays on. */
  readonly domain: string;
  /** The reader's own account, to leave out of a reply's mentions; undefined until it is read. */
  readonly viewer: { readonly id: string; readonly acct: string } | undefined;
  /** How near and how far ahead a post may be scheduled (ADR-0156 §7). */
  readonly schedule: { readonly minLeadMinutes: number; readonly maxLeadDays: number };
};

/** The settings until the server's are read: the server's defaults. */
export const defaultComposeSettings: ComposeSettings = {
  maxChars: 500,
  requireDescriptions: false,
  visibility: "public",
  policy: {
    reply: { audience: "everyone", approval: false },
    announce: { audience: "everyone", approval: false },
    quote: { audience: "everyone", approval: false },
  },
  language: "und",
  domain: "",
  viewer: undefined,
  schedule: {
    minLeadMinutes: defaultDraftLimits.scheduled_post_min_lead_minutes,
    maxLeadDays: defaultDraftLimits.scheduled_post_max_lead_days,
  },
};

/** The settings a new post starts from: the reader's preferences and the instance's limits, the defaults until read. */
export function useComposeSettings(): ComposeSettings {
  const { queries } = useAppData();
  const preferences = useQuery(queries.queryOptions("get", "/preferences")).data;
  const instance = useQuery(queries.queryOptions("get", "/instance")).data;
  const viewer = useQuery(queries.queryOptions("get", "/accounts/verify_credentials")).data;
  const policy = preferences?.default_interaction_policy;
  return {
    maxChars: instance?.limits.max_post_characters ?? defaultComposeSettings.maxChars,
    requireDescriptions:
      (instance?.limits.require_media_description ?? false) || (preferences?.require_media_description ?? false),
    visibility: preferences?.default_visibility ?? defaultComposeSettings.visibility,
    policy: policy
      ? { reply: choiceOf(policy.reply), announce: choiceOf(policy.announce), quote: choiceOf(policy.quote) }
      : defaultComposeSettings.policy,
    language: preferences?.default_post_language ?? defaultComposeSettings.language,
    domain: instance?.domain ?? defaultComposeSettings.domain,
    viewer: viewer && { id: viewer.id, acct: viewer.acct },
    schedule: {
      minLeadMinutes:
        instance?.limits.drafts?.scheduled_post_min_lead_minutes ?? defaultComposeSettings.schedule.minLeadMinutes,
      maxLeadDays: instance?.limits.drafts?.scheduled_post_max_lead_days ?? defaultComposeSettings.schedule.maxLeadDays,
    },
  };
}

/**
 * A status by its ID as it shows, at once from a cached query that holds it, else from the server; none without an ID,
 * or once it is gone.
 */
export function useStatus(id: string | undefined): { status: Status | undefined; failed: boolean } {
  const data = useAppData();
  const show = useShadow();
  const options = data.queries.queryOptions("get", "/statuses/{id}", { params: { path: { id: id ?? "" } } });
  const query = useQuery({
    ...options,
    enabled: id !== undefined,
    initialData: () => (id === undefined ? undefined : findCachedCopy(data.queryClient, id)),
  });
  return { status: query.data && show(query.data), failed: query.isError };
}

/**
 * Posts a status with `idempotencyKey`, the same for every attempt of one post, so that a retry after a lost answer
 * posts it once; the posted status goes into the cache.
 */
export async function postStatus(data: AppData, body: CreateStatus, idempotencyKey: string): Promise<Status> {
  const { data: status } = await data.api.POST("/statuses", {
    params: { header: { "Idempotency-Key": idempotencyKey } },
    body,
  });
  if (status === undefined) throw new Error("app-data: the post answered without a status");
  cachePostedStatus(data, status);
  return status;
}

/**
 * Writes what a posted status makes certain: it tops the home timeline, and the post it replies to has one more reply
 * than it showed, which its cached threads show; a copy the server answered with the reply counted already keeps its
 * count, and a reply that waits for the approval of its parent's author counts nowhere yet (its author alone sees it).
 * A direct reply is the latest post of its conversation (conversations.ts).
 */
export function cachePostedStatus(data: AppData, status: Status): void {
  prependHomeStatuses(data, [status]);
  const parent = status.in_reply_to_id;
  if (parent === undefined) return;
  cacheConversationPost(data, status);
  const shown = cachedStatus(data, parent);
  if (shown !== undefined && status.approval === undefined) {
    const count = shown.replies_count + 1;
    shadowStatus(data, parent, (s) => ({ ...s, replies_count: Math.max(s.replies_count, count) }), "replies").confirm();
  }
  cacheReply(data.queryClient, status);
}

/** How long to wait between two looks at a file the server processes. */
const processingPollMs = 1000;

/** A media file the server could not process. */
export class MediaFailedError extends Error {
  override readonly name = "MediaFailedError";
}

/**
 * Uploads a media file and waits until the server has processed it: `onProcessing` hears when the upload is done and
 * the processing starts. It resolves with the ready file, and rejects with MediaFailedError when the server could not
 * process it, or with mykit's error when a request failed.
 */
export async function uploadAttachment(
  data: AppData,
  file: File,
  { signal, onProcessing }: { signal?: AbortSignal | undefined; onProcessing?: (() => void) | undefined } = {},
): Promise<MediaAttachment> {
  let media = await uploadMedia(data.api, { file }, signal === undefined ? undefined : { signal });
  if (media.state === "processing") onProcessing?.();
  while (media.state === "processing") {
    await new Promise((resolve) => setTimeout(resolve, processingPollMs));
    signal?.throwIfAborted();
    const { data: next } = await data.api.GET("/media/{id}", {
      params: { path: { id: media.id } },
      ...(signal === undefined ? {} : { signal }),
    });
    if (next === undefined) throw new Error("app-data: the media answered without a file");
    media = next;
  }
  if (media.state === "failed") throw new MediaFailedError(`app-data: the server could not process ${media.id}`);
  return media;
}

/**
 * Saves a draft on the server (ADR-0156 §9): over the draft saved before, or as a new one when there is none. When
 * another device saved over it since (412), or it is gone (404), it is saved as a new one, so that nothing is lost
 * (`onConflict: "new"`, a draft left by accident, which publishes nothing), or refused, so that the reader chooses
 * (`"refuse"`, the composer's own saves, which may publish or schedule it): with DraftConflictError, or with
 * DraftGoneError for a draft that may have been published already, which a new one would publish again.
 */
export async function saveDraft(
  data: AppData,
  body: DraftRequest,
  saved?: SavedDraft,
  { onConflict = "new" }: { onConflict?: "new" | "refuse" } = {},
): Promise<SavedDraft> {
  try {
    if (saved !== undefined) {
      try {
        const { data: draft, response } = await data.api.PUT("/drafts/{id}", {
          params: { path: { id: saved.id }, header: { "If-Match": saved.etag } },
          body,
        });
        return savedOf(draft, response);
      } catch (e) {
        if (e instanceof ProblemError && e.status === 412 && onConflict === "refuse")
          throw new DraftConflictError(`app-data: the draft ${saved.id} was saved elsewhere since`);
        if (e instanceof ProblemError && e.status === 404 && onConflict === "refuse")
          throw new DraftGoneError(`app-data: the draft ${saved.id} is gone`);
        if (!(e instanceof ProblemError && (e.status === 412 || e.status === 404))) throw e;
      }
    }
    const { data: draft, response } = await data.api.POST("/drafts", { body });
    return savedOf(draft, response);
  } finally {
    void invalidateDrafts(data);
  }
}

/**
 * Publishes the draft of `id` now, as the server holds it (ADR-0156 §9): the posted status goes into the cache, and
 * the draft is gone. One gone already (404) rejects with DraftGoneError: it may have been published.
 */
export async function publishDraft(data: AppData, id: string): Promise<Status> {
  try {
    const { data: status } = await data.api.POST("/drafts/{id}/publish", { params: { path: { id } } });
    if (status === undefined) throw new Error("app-data: the draft's publication answered without a status");
    cachePostedStatus(data, status);
    return status;
  } catch (e) {
    if (e instanceof ProblemError && e.status === 404) throw new DraftGoneError(`app-data: the draft ${id} is gone`);
    throw e;
  } finally {
    void invalidateDrafts(data);
  }
}

/** A media file of the reader's as the server holds it now: one of a draft, which keeps only its ID. */
export async function readMedia(data: AppData, id: string): Promise<MediaAttachment> {
  const { data: media } = await data.api.GET("/media/{id}", { params: { path: { id } } });
  if (media === undefined) throw new Error("app-data: the media answered without a file");
  return media;
}

/**
 * What the server said of a refused write, in the reader's language (it localizes from Accept-Language): the
 * problem's detail, else its title; undefined for a failure without an answer (the network), which the caller words.
 */
export function refusal(error: unknown): string | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  return error.detail !== undefined && error.detail !== "" ? error.detail : error.title || undefined;
}

/**
 * The member of the request a refused write is about, when the server named one (a 422's `field`: `text`,
 * `poll.options[1]`, `Idempotency-Key`), with what is wrong with its value (the problem's detail, which never names
 * it) and the problem's title, to say once for the whole form; undefined for any other failure.
 */
export function refusedField(error: unknown): { member: string; detail: string; title: string } | undefined {
  if (!(error instanceof ProblemError) || error.status !== 422) return undefined;
  const member = error.problem.field;
  const detail = error.detail;
  if (member === undefined || member === "" || detail === undefined || detail === "") return undefined;
  return { member, detail, title: error.title };
}

/** The writes of the composer, bound to the session's data. */
export type ComposeWrites = {
  readonly post: (body: CreateStatus, idempotencyKey: string) => Promise<Status>;
  /** Edits the reader's post of `id` (own-posts.ts). */
  readonly edit: (id: string, body: EditStatus) => Promise<Status>;
  readonly upload: (file: File, options?: Parameters<typeof uploadAttachment>[2]) => Promise<MediaAttachment>;
  /** Reads a media file of the reader's (a draft's). */
  readonly media: (id: string) => Promise<MediaAttachment>;
  readonly saveDraft: (
    body: DraftRequest,
    saved?: SavedDraft,
    options?: Parameters<typeof saveDraft>[3],
  ) => Promise<SavedDraft>;
  readonly deleteDraft: (id: string) => Promise<void>;
  readonly readDraft: (id: string) => Promise<{ draft: ServerDraft; saved: SavedDraft }>;
  /** Schedules the draft of `id` at a time (ISO 8601), or moves its schedule there. */
  readonly schedule: (id: string, publishAt: string) => Promise<SavedDraft>;
  readonly unschedule: (id: string) => Promise<SavedDraft>;
  /** Publishes the draft of `id` now. */
  readonly publish: (id: string) => Promise<Status>;
};

/** The composer's writes, for the session of the nearest AppDataProvider. */
export function useComposeWrites(): ComposeWrites {
  const data = useAppData();
  return {
    post: (body, key) => postStatus(data, body, key),
    edit: (id, body) => editStatus(data, id, body),
    upload: (file, options) => uploadAttachment(data, file, options),
    media: (id) => readMedia(data, id),
    saveDraft: (body, saved, options) => saveDraft(data, body, saved, options),
    deleteDraft: (id) => deleteDraft(data, id),
    readDraft: (id) => readDraft(data, id),
    schedule: (id, publishAt) => scheduleDraft(data, id, publishAt),
    unschedule: (id) => unscheduleDraft(data, id),
    publish: (id) => publishDraft(data, id),
  };
}

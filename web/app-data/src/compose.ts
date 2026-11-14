// SPDX-License-Identifier: AGPL-3.0-only
// What the post composer reads and writes (ADR-0028 §6, §7, ADR-0156): the settings a new post starts from, the post
// replied to or quoted, the media uploads, the drafts kept on the server, and the post itself. A posted status goes
// into the cache at once: at the top of the home timeline, and one more reply on the post it replies to, shown in its
// cached threads. The write is a plain request with the composer's Idempotency-Key (no outbox yet, #2892): a failure
// rejects with mykit's ProblemError or NetworkError, and the composer keeps what was written to send again with the
// same key.
import { ProblemError, uploadMedia, type components, type MediaAttachment } from "@mycroblog/mykit";
import { useQuery, type QueryClient } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { prependHomeStatuses } from "./arrivals";
import { useAppData } from "./provider";
import { updateCachedStatuses, type Status, type StatusPage } from "./statuses";
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

/** What a new post starts from, and what the server allows of it. */
export type ComposeSettings = {
  /** The most characters of the text and the content warning together. */
  readonly maxChars: number;
  /** Whether every image needs a description before it is posted. */
  readonly requireDescriptions: boolean;
  readonly visibility: Visibility;
  /** Whom each interaction admits at once, by default. */
  readonly policy: Readonly<Record<Interaction, Audience>>;
  /** The default language of a post, a BCP 47 tag; `und` when there is none. */
  readonly language: string;
  /** The instance's domain, which a local post stays on. */
  readonly domain: string;
  /** The reader's own account, to leave out of a reply's mentions; undefined until it is read. */
  readonly viewer: { readonly id: string; readonly acct: string } | undefined;
};

/** The settings until the server's are read: the server's defaults. */
export const defaultComposeSettings: ComposeSettings = {
  maxChars: 500,
  requireDescriptions: false,
  visibility: "public",
  policy: { reply: "everyone", announce: "everyone", quote: "everyone" },
  language: "und",
  domain: "",
  viewer: undefined,
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
      ? { reply: policy.reply.automatic, announce: policy.announce.automatic, quote: policy.quote.automatic }
      : defaultComposeSettings.policy,
    language: preferences?.default_post_language ?? defaultComposeSettings.language,
    domain: instance?.domain ?? defaultComposeSettings.domain,
    viewer: viewer && { id: viewer.id, acct: viewer.acct },
  };
}

/** The status with the ID given as some cached timeline holds it (or boosts or quotes it), else undefined. */
export function findCachedStatus(queryClient: QueryClient, id: string): Status | undefined {
  const find = (status: Status): Status | undefined => {
    if (status.id === id) return status;
    const inner = status.boost ?? status.quote?.quoted_status;
    return inner && find(inner);
  };
  for (const [key, data] of queryClient.getQueriesData<{ pages: StatusPage[] }>({ queryKey: ["get"] })) {
    if (typeof key[1] !== "string" || !key[1].startsWith("/timelines/") || !data?.pages) continue;
    for (const page of data.pages)
      for (const status of page.items) {
        const found = find(status);
        if (found) return found;
      }
  }
  return undefined;
}

/** A status by its ID, at once from a cached timeline that holds it, else from the server; none without an ID. */
export function useStatus(id: string | undefined): { status: Status | undefined; failed: boolean } {
  const data = useAppData();
  const options = data.queries.queryOptions("get", "/statuses/{id}", { params: { path: { id: id ?? "" } } });
  const query = useQuery({
    ...options,
    enabled: id !== undefined,
    initialData: () => (id === undefined ? undefined : findCachedStatus(data.queryClient, id)),
  });
  return { status: query.data, failed: query.isError };
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
 * Writes what a posted status makes certain: it tops the home timeline, and the post it replies to has one more reply,
 * which its cached threads show.
 */
export function cachePostedStatus(data: AppData, status: Status): void {
  prependHomeStatuses(data, [status]);
  const parent = status.in_reply_to_id;
  if (parent === undefined) return;
  updateCachedStatuses(data.queryClient, parent, (s) => ({ ...s, replies_count: s.replies_count + 1 }));
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

/** A draft as saved: its ID, and the ETag the next save sends to replace it. */
export type SavedDraft = { readonly id: string; readonly etag: string };

/**
 * Saves a draft on the server (ADR-0156 §9): over the draft saved before, or as a new one when there is none, or when
 * another device saved over it since (412), so that neither is lost.
 */
export async function saveDraft(data: AppData, body: DraftRequest, saved?: SavedDraft): Promise<SavedDraft> {
  if (saved !== undefined) {
    try {
      const { data: draft, response } = await data.api.PUT("/drafts/{id}", {
        params: { path: { id: saved.id }, header: { "If-Match": saved.etag } },
        body,
      });
      if (draft === undefined) throw new Error("app-data: the draft answered without a draft");
      return { id: draft.id, etag: response.headers.get("ETag") ?? "" };
    } catch (e) {
      if (!(e instanceof ProblemError && (e.status === 412 || e.status === 404))) throw e;
    }
  }
  const { data: draft, response } = await data.api.POST("/drafts", { body });
  if (draft === undefined) throw new Error("app-data: the draft answered without a draft");
  return { id: draft.id, etag: response.headers.get("ETag") ?? "" };
}

/**
 * What the server said of a refused write, in the reader's language (it localizes from Accept-Language): the
 * problem's detail, else its title; undefined for a failure without an answer (the network), which the caller words.
 */
export function refusal(error: unknown): string | undefined {
  if (!(error instanceof ProblemError)) return undefined;
  return error.detail !== undefined && error.detail !== "" ? error.detail : error.title || undefined;
}

/** The writes of the composer, bound to the session's data. */
export type ComposeWrites = {
  readonly post: (body: CreateStatus, idempotencyKey: string) => Promise<Status>;
  readonly upload: (file: File, options?: Parameters<typeof uploadAttachment>[2]) => Promise<MediaAttachment>;
  readonly saveDraft: (body: DraftRequest, saved?: SavedDraft) => Promise<SavedDraft>;
  readonly deleteDraft: (id: string) => Promise<void>;
};

/** The composer's writes, for the session of the nearest AppDataProvider. */
export function useComposeWrites(): ComposeWrites {
  const data = useAppData();
  return {
    post: (body, key) => postStatus(data, body, key),
    upload: (file, options) => uploadAttachment(data, file, options),
    saveDraft: (body, saved) => saveDraft(data, body, saved),
    deleteDraft: (id) => deleteDraft(data, id),
  };
}

/** Deletes a draft; one already gone is no failure. */
export async function deleteDraft(data: AppData, id: string): Promise<void> {
  try {
    await data.api.DELETE("/drafts/{id}", { params: { path: { id } } });
  } catch (e) {
    if (!(e instanceof ProblemError && e.status === 404)) throw e;
  }
}

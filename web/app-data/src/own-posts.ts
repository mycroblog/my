// SPDX-License-Identifier: AGPL-3.0-only
// The reader's own posts (FR-108, FR-109, ADR-0028 §7, §18): deleting one, deleting it to write it again (redraft),
// editing one from what its author typed, and the edit history of any post. A deletion goes through the shadow: the
// post leaves every list and view at once, and comes back if the server refuses or does not answer. An edit shows on
// every copy once the server answered, since only the server renders a post's text; its history is read again then. A
// redraft reads the post's source before it deletes it, since the deletion answers nothing. The writes are plain
// requests (no outbox yet, #2892): a failure rejects with mykit's ProblemError or NetworkError.
import { ProblemError, type components, type MediaAttachment } from "@mycroblog/mykit";
import { useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { useAppData } from "./provider";
import { cachedStatus, changeStatus, receiveGone, removeStatus } from "./shadow";
import type { Status } from "./statuses";

/** The new content of the reader's post, as the API takes it. */
export type EditStatus = components["schemas"]["EditStatusRequest"];

/** One version of a post (the API's StatusEdit). */
export type StatusEdit = components["schemas"]["StatusEdit"];

/**
 * What the reader's post holds, to write it again: what its author typed (the API's StatusSource), and from the post
 * itself its audience, its images, its poll and the posts it replies to and quotes.
 */
export type PostSource = {
  readonly id: string;
  readonly text: string;
  /** The content warning; empty when there is none. */
  readonly spoiler_text: string;
  /** A BCP 47 tag; `und` when unknown. */
  readonly language: string;
  readonly visibility: Status["visibility"];
  readonly sensitive: boolean;
  readonly media: readonly MediaAttachment[];
  /** Its poll's options and how it counts, and how long it ran (seconds) when it had an end. */
  readonly poll?: { readonly options: readonly string[]; readonly multiple: boolean; readonly duration?: number };
  readonly in_reply_to_id?: string | undefined;
  readonly quoted_status_id?: string | undefined;
};

/** The status of `id` as it shows, from the cache, else from the server. */
async function statusOf(data: AppData, id: string): Promise<Status> {
  const cached = cachedStatus(data, id);
  if (cached !== undefined) return cached;
  const { data: status } = await data.api.GET("/statuses/{id}", { params: { path: { id } } });
  if (status === undefined) throw new Error("app-data: the status answered without a status");
  return status;
}

/** What the reader's post of `id` holds, to edit it or write it again; it rejects when either read fails. */
export async function readPostSource(data: AppData, id: string): Promise<PostSource> {
  const [{ data: source }, status] = await Promise.all([
    data.api.GET("/statuses/{id}/source", { params: { path: { id } } }),
    statusOf(data, id),
  ]);
  if (source === undefined) throw new Error("app-data: the source answered without a source");
  const poll = status.poll;
  const ends = poll?.expires_at === undefined ? undefined : Date.parse(poll.expires_at);
  const duration = ends === undefined ? undefined : Math.round((ends - Date.parse(status.created_at)) / 1000);
  const quoted = status.quote?.quoted_status?.id;
  return {
    id,
    text: source.text,
    spoiler_text: source.spoiler_text,
    language: source.language,
    visibility: status.visibility,
    sensitive: status.sensitive,
    media: status.media_attachments,
    ...(poll === undefined
      ? {}
      : {
          poll: {
            options: poll.options.map((o) => o.title),
            multiple: poll.multiple,
            ...(duration === undefined ? {} : { duration }),
          },
        }),
    in_reply_to_id: status.in_reply_to_id,
    quoted_status_id: quoted,
  };
}

/** Deletes the reader's post of `id`: it leaves every list and view at once, and comes back if the deletion fails. */
export async function deleteStatus(data: AppData, id: string): Promise<void> {
  await removeStatus(data, id, () => data.api.DELETE("/statuses/{id}", { params: { path: { id } } }));
}

/** A redraft whose deletion failed after the post's source was read: the source, to delete again without a read. */
export class RedraftError extends Error {
  override readonly name = "RedraftError";
  readonly source: PostSource;
  constructor(source: PostSource, options: { cause: unknown }) {
    super("app-data: the redraft's deletion failed", options);
    this.source = source;
  }
}

/**
 * Deletes the reader's post of `id` to write it again: it resolves with what the post held, read before it is deleted,
 * or `source` when given (that of an earlier attempt, since a post deleted may not be read again). It rejects, and the
 * post stays, when the read fails, or with a RedraftError carrying the source when the deletion fails; a deletion the
 * server answers that the post is not found finds it deleted already (an earlier answer lost), and it resolves.
 */
export async function redraftStatus(data: AppData, id: string, source?: PostSource): Promise<PostSource> {
  const read = source ?? (await readPostSource(data, id));
  try {
    await deleteStatus(data, id);
  } catch (e) {
    if (!(e instanceof ProblemError && e.status === 404)) throw new RedraftError(read, { cause: e });
    receiveGone(data, id);
  }
  return read;
}

/** The fields of a status that an edit changes, as the server answered them. */
const editedFields = (s: Status): Partial<Status> => ({
  content: s.content,
  spoiler_text: s.spoiler_text,
  sensitive: s.sensitive,
  language: s.language,
  media_attachments: s.media_attachments,
  mentions: s.mentions,
  tags: s.tags,
  emojis: s.emojis,
  ...(s.edited_at === undefined ? {} : { edited_at: s.edited_at }),
  ...(s.card === undefined ? {} : { card: s.card }),
  ...(s.quote === undefined ? {} : { quote: s.quote }),
});

/** The edit history of the status of `id`, oldest first. */
export function statusHistoryQuery({ queries }: AppData, id: string) {
  return queries.queryOptions("get", "/statuses/{id}/history", { params: { path: { id } } });
}

/**
 * Edits the reader's post of `id`: once the server took the edit, every copy shows the post as the server answered,
 * and its edit history is read again; it rejects with the server's refusal or the network's failure, and the copies
 * stay as they were.
 */
export async function editStatus(data: AppData, id: string, body: EditStatus): Promise<Status> {
  const answer = await changeStatus(data, id, {
    key: "content",
    change: (s) => s,
    send: async () => (await data.api.PUT("/statuses/{id}", { params: { path: { id } }, body })).data,
    answered: (a) => {
      if (a?.id !== id) return undefined;
      const after = editedFields(a);
      // The answer's link card and quote replace the post's, none of them too.
      return ({ card: _card, quote: _quote, ...s }) => ({ ...s, ...after });
    },
  });
  if (answer === undefined) throw new Error("app-data: the edit answered without a status");
  await data.queryClient.invalidateQueries({ queryKey: statusHistoryQuery(data, id).queryKey });
  return answer;
}

/** The edit history of the status of `id`, oldest first, from the cache unless it is stale; it rejects with the read. */
export async function readStatusHistory(data: AppData, id: string): Promise<readonly StatusEdit[]> {
  return (await data.queryClient.query(statusHistoryQuery(data, id))).items;
}

/** The edit history of the status of `id` (none for undefined), oldest first; undefined until the server answered. */
export function useStatusHistory(id: string | undefined): readonly StatusEdit[] | undefined {
  const data = useAppData();
  return useQuery({ ...statusHistoryQuery(data, id ?? ""), enabled: id !== undefined }).data?.items;
}

/** What the reader does to their own posts, and a post's edit history, for the session of the nearest AppDataProvider. */
export type OwnPosts = {
  readonly source: (id: string) => Promise<PostSource>;
  readonly remove: (id: string) => Promise<void>;
  /** Deletes the post to write it again; `source`, when given, is that of an attempt whose deletion failed. */
  readonly redraft: (id: string, source?: PostSource) => Promise<PostSource>;
  readonly history: (id: string) => Promise<readonly StatusEdit[]>;
  /** The status of `id` as it shows, from a cached copy; undefined when none is cached, or it is gone. */
  readonly cached: (id: string) => Status | undefined;
};

export function useOwnPosts(): OwnPosts {
  const data = useAppData();
  return {
    source: (id) => readPostSource(data, id),
    remove: (id) => deleteStatus(data, id),
    redraft: (id, source) => redraftStatus(data, id, source),
    history: (id) => readStatusHistory(data, id),
    cached: (id) => cachedStatus(data, id),
  };
}

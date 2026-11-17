// SPDX-License-Identifier: AGPL-3.0-only
// What the article editor reads and writes (FR-122, FR-123, ADR-0160 §3, §5, §8): whether the reader may write new
// articles, the limits of an article's body, the preview of a body as the server would publish it, a new article, the
// edit of one, and what its author wrote of it. A posted article goes into the cache as a post does (compose.ts); an
// edited one shows on every copy once the server answered, since only the server renders its body. Drafts of articles
// are drafts (drafts.ts, compose.ts), with their `article`.
import type { components } from "@mycroblog/mykit";
import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AppData } from "./app-data";
import { cachePostedStatus } from "./compose";
import { useAppData } from "./provider";
import { changeStatus } from "./shadow";
import type { Status } from "./statuses";

/** A new article, as the API takes it. */
export type CreateArticle = components["schemas"]["CreateArticleRequest"];

/** The new content of an article, as the API takes it. */
export type EditArticle = components["schemas"]["EditArticleRequest"];

/** A body as publishing it would store it: its HTML, the summary it would get, and its size and images with their limits. */
export type ArticlePreview = components["schemas"]["ArticlePreview"];

/** What the author wrote of an article (the API's StatusSource with its `article`). */
export type ArticleSource = components["schemas"]["StatusSource"];

/** The limits of an article's body: its HTML's bytes and its images. */
export type ArticleLimits = components["schemas"]["InstanceArticleLimits"];

/** The limits until the instance's are read: the server's defaults (ADR-0160 §5). */
export const defaultArticleLimits: ArticleLimits = { max_bytes: 100_000, max_images: 20 };

/** The limits of the title, the summary and the hashtags, which the instance does not change (ADR-0160 §5). */
export const articleFieldLimits = { title: 200, summary: 500, tags: 10 } as const;

/**
 * Whether the reader may write new articles: everyone may, unless the instance keeps them to the permission
 * `create_articles` (ADR-0160 §3); undefined until both the instance and the reader's account are read, or without a
 * session (`enabled` false).
 */
export function useArticleAuthoring(enabled = true): boolean | undefined {
  const { queries } = useAppData();
  const instance = useQuery({ ...queries.queryOptions("get", "/instance"), enabled }).data;
  const viewer = useQuery({ ...queries.queryOptions("get", "/accounts/verify_credentials"), enabled }).data;
  if (!enabled || instance === undefined) return undefined;
  if (instance.article_authoring !== "permission") return true;
  return viewer === undefined ? undefined : viewer.permissions.includes("create_articles");
}

/** The limits of an article's body on this server, its defaults until read. */
export function useArticleLimits(): ArticleLimits {
  const { queries } = useAppData();
  return useQuery(queries.queryOptions("get", "/instance")).data?.limits.articles ?? defaultArticleLimits;
}

/** Renders a body (and its summary) as publishing it would, storing nothing. */
export async function previewArticle(
  data: AppData,
  body: { text: string; summary: string },
  signal?: AbortSignal,
): Promise<ArticlePreview> {
  const { data: preview } = await data.api.POST("/articles/preview", {
    body: body.summary === "" ? { text: body.text } : body,
    ...(signal === undefined ? {} : { signal }),
  });
  if (preview === undefined) throw new Error("app-data: the preview answered without a body");
  return preview;
}

/** The preview of a body: the latest answer, shown while the next renders, and whether the last request failed. */
export type ArticlePreviewState = {
  readonly preview: ArticlePreview | undefined;
  /** A request is on its way. */
  readonly busy: boolean;
  /** The last request failed (the preview shown, if any, is of an earlier body). */
  readonly failed: boolean;
  readonly retry: () => void;
  /** The body and summary the preview shown is of; none while it is an earlier body's, or there is none. */
  readonly of: { readonly text: string; readonly summary: string } | undefined;
  /**
   * The preview of `text` with `summary` now, from the cache when it holds it, else asked for at once; it rejects when
   * the server could not render it.
   */
  readonly now: (text: string, summary: string) => Promise<ArticlePreview>;
};

/** The query of the preview of a body and its summary. */
function previewQuery(data: AppData, text: string, summary: string) {
  return {
    queryKey: ["post", "/articles/preview", { text, summary }] as const,
    queryFn: ({ signal }: { signal: AbortSignal }) => previewArticle(data, { text, summary }, signal),
    staleTime: Number.POSITIVE_INFINITY,
    retry: false,
  };
}

/**
 * The preview of `text` with `summary`, from the server, once per body (the caller waits for the writer to pause
 * before it asks for the next); none for an empty body, which the server would not publish.
 */
export function useArticlePreview(text: string, summary: string): ArticlePreviewState {
  const data = useAppData();
  const query = useQuery({
    ...previewQuery(data, text, summary),
    enabled: text.trim() !== "",
    placeholderData: keepPreviousData,
  });
  const empty = text.trim() === "";
  return {
    preview: empty ? undefined : query.data,
    busy: query.isFetching,
    failed: query.isError,
    retry: () => {
      void query.refetch();
    },
    of: empty || query.data === undefined || query.isPlaceholderData ? undefined : { text, summary },
    now: (t, s) => data.queryClient.query(previewQuery(data, t, s)),
  };
}

/**
 * Posts an article with `idempotencyKey`, the same for every attempt of one article, so that a retry after a lost
 * answer posts it once; the posted article goes into the cache as a post does.
 */
export async function postArticle(data: AppData, body: CreateArticle, idempotencyKey: string): Promise<Status> {
  const { data: status } = await data.api.POST("/articles", {
    params: { header: { "Idempotency-Key": idempotencyKey } },
    body,
  });
  if (status === undefined) throw new Error("app-data: the article answered without a status");
  cachePostedStatus(data, status);
  return status;
}

/**
 * Edits the reader's article of `id`: once the server took the edit, every copy shows the article as the server
 * answered; it rejects with the server's refusal or the network's failure, and the copies stay as they were.
 */
export async function editArticle(data: AppData, id: string, body: EditArticle): Promise<Status> {
  return changeStatus(data, id, {
    key: "content",
    change: (s) => s,
    send: async () => {
      const { data: status } = await data.api.PUT("/articles/{id}", { params: { path: { id } }, body });
      if (status === undefined) throw new Error("app-data: the edit answered without a status");
      return status;
    },
    answered: (a) => {
      if (a.id !== id) return undefined;
      return (s) => ({
        ...s,
        // A copy in a list holds no body (ADR-0160 §8); the one read alone holds the new one.
        content: s.content === "" ? "" : a.content,
        sensitive: a.sensitive,
        language: a.language,
        tags: a.tags,
        emojis: a.emojis,
        ...(a.article === undefined ? {} : { article: a.article }),
        ...(a.edited_at === undefined ? {} : { edited_at: a.edited_at }),
      });
    },
  });
}

/** What the reader wrote of their article of `id`, to edit it. */
export async function readArticleSource(data: AppData, id: string): Promise<ArticleSource> {
  const { data: source } = await data.api.GET("/statuses/{id}/source", { params: { path: { id } } });
  if (source === undefined) throw new Error("app-data: the source answered without a source");
  return source;
}

/** The query of what the reader wrote of their article of `id`, which the editor of it starts from. */
export function articleSourceQuery(data: AppData, id: string) {
  return {
    queryKey: ["get", "/statuses/{id}/source", { params: { path: { id } } }] as const,
    queryFn: () => readArticleSource(data, id),
    staleTime: 0,
  };
}

/** What the reader wrote of their article of `id`, and whether it could not be read; nothing without an ID. */
export function useArticleSource(id: string | undefined): {
  source: ArticleSource | undefined;
  failed: boolean;
  retry: () => void;
} {
  const data = useAppData();
  const query = useQuery({ ...articleSourceQuery(data, id ?? ""), enabled: id !== undefined });
  return {
    source: query.data,
    failed: query.isError,
    retry: () => {
      void query.refetch();
    },
  };
}

/** The article editor's writes, bound to the session's data. */
export type ArticleWrites = {
  readonly post: (body: CreateArticle, idempotencyKey: string) => Promise<Status>;
  readonly edit: (id: string, body: EditArticle) => Promise<Status>;
};

/** The article editor's writes, for the session of the nearest AppDataProvider. */
export function useArticleWrites(): ArticleWrites {
  const data = useAppData();
  return {
    post: (body, key) => postArticle(data, body, key),
    edit: (id, body) => editArticle(data, id, body),
  };
}

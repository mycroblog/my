// SPDX-License-Identifier: AGPL-3.0-only
// The server data of the article editor (ADR-0160 §8, web-client/screens/article-editor.md): the preview of the body
// rendered by the server (POST /articles/preview, which stores nothing), a draft of an article with the ETag of its
// next save, what the author wrote of a published article, and the edit of a published article. A new article is
// saved and published through the drafts (compose's api/drafts.ts), so that it is published once. Types come from the
// spec (S9).
import { queryOptions } from "@tanstack/react-query";
import type { components } from "@mycroblog/mykit";
import type { Api } from "../../../lib/api/client";
import { ApiError } from "../../../lib/errors";
import type { DraftVersion } from "../../compose";

export type ArticlePreviewRequest = components["schemas"]["ArticlePreviewRequest"];
export type EditArticleRequest = components["schemas"]["EditArticleRequest"];
export type StatusSource = components["schemas"]["StatusSource"];

/**
 * The body rendered as publishing it would store it (POST /articles/preview): the HTML, the summary, and the size and
 * images with their limits. It is a read, so it is a query of the request: the same text is rendered once, and again
 * when `images`, the uploads of the body that are ready, changes (the server shows an image once it is processed).
 */
export function articlePreviewQueryOptions(api: Api, body: ArticlePreviewRequest, images: string) {
  return queryOptions({
    queryKey: ["post", "/articles/preview", body, images] as const,
    queryFn: async ({ signal }) => {
      const { data, error } = await api.fetch.POST("/articles/preview", { body, signal });
      // The client's middleware throws an ApiError for every error status (lib/api/client.ts).
      if (data === undefined) throw new ApiError(error);
      return data;
    },
    // A preview never changes for the same request; it is kept only while the text is on screen.
    staleTime: Infinity,
    gcTime: 60_000,
    retry: false,
  });
}

/**
 * A draft of an article with the ETag of its next save (GET /drafts/{id}). It is always stale, so that the route's
 * loader reads it afresh each time the editor opens it: another device may have saved it since. The editor reads the
 * loader's answer once, as what it starts from.
 */
export function articleDraftQueryOptions(api: Api, id: string) {
  return queryOptions({
    queryKey: ["article-editor", "draft", id] as const,
    queryFn: async ({ signal }): Promise<DraftVersion> => {
      const { data, error, response } = await api.fetch.GET("/drafts/{id}", { params: { path: { id } }, signal });
      if (data === undefined) throw new ApiError(error);
      const etag = response.headers.get("ETag");
      if (etag === null) throw new Error("the draft answered without an ETag");
      return { draft: data, etag };
    },
    staleTime: 0,
    retry: false,
  });
}

/** What the author wrote of a published article, to edit it (GET /statuses/{id}/source); as the draft, always stale. */
export function articleSourceQueryOptions(api: Api, id: string) {
  return api.query.queryOptions(
    "get",
    "/statuses/{id}/source",
    { params: { path: { id } } },
    { staleTime: 0, retry: false },
  );
}

/** The published article, for its visibility, sensitivity, and cover. */
export function articleStatusQueryOptions(api: Api, id: string) {
  return api.query.queryOptions("get", "/statuses/{id}", { params: { path: { id } } });
}

/** Replaces the content of the published article `id` (PUT /articles/{id}); the answer is the edited article. */
export async function editArticle(api: Api, id: string, body: EditArticleRequest) {
  const { data, error } = await api.fetch.PUT("/articles/{id}", { params: { path: { id } }, body });
  if (data === undefined) throw new ApiError(error);
  return data;
}

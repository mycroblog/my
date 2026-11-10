// SPDX-License-Identifier: AGPL-3.0-only
// The default MSW handlers of the article editor (web-client/README.md §7): the preview of the body, which renders
// each paragraph and each image of an upload (`![alt](media:ID)`) the way the server does, and the edit of a published
// article. A new article is saved and published through the drafts, whose handlers are compose's.
import { http, HttpResponse } from "msw";
import { mediaFixture, type Schemas, viewerStatusFixture } from "../../../test/fixtures";
import { apiBase, authorized, unauthenticated, withViewer } from "../../../test/msw/http";

/** The ID of the viewer's article that the tests edit. */
export const articleId = "0192f0c4-7a8e-7000-8000-0000000005a1";

/** The most bytes and images of a body that the preview reports. */
const limits = { max_bytes: 100_000, max_images: 20 };

/** The text escaped for HTML. */
function escaped(text: string): string {
  return text.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;").replaceAll('"', "&quot;");
}

/**
 * The body rendered as the server would, for the tests: a paragraph per block, an image of an upload as its `large`
 * rendition, and `**bold**`. It is not Markdown; it only lets the tests see what the preview shows.
 */
export function renderedBody(text: string): { content: string; images: number } {
  let images = 0;
  const blocks = text
    .split(/\n{2,}/u)
    .map((block) => block.trim())
    .filter((block) => block !== "")
    .map((block) => {
      const image = /^!\[([^\]]*)\]\(media:([^)]+)\)$/u.exec(block);
      if (image !== null) {
        images += 1;
        const [, alt = "", id = ""] = image;
        return `<p><img src="${mediaFixture(id).variants?.large.url ?? ""}" alt="${escaped(alt)}" loading="lazy"></p>`;
      }
      return `<p>${escaped(block).replaceAll(/\*\*([^*]+)\*\*/gu, "<strong>$1</strong>")}</p>`;
    });
  return { content: blocks.join(""), images };
}

/** The viewer's article with `article` and the body `text`, as the server answers it. */
export function articleStatus(article: Schemas["StatusArticle"], text: string): Schemas["Status"] {
  return viewerStatusFixture({ id: articleId, article, content: renderedBody(text).content });
}

export const handlers = [
  // operationId previewArticle.
  http.post(`${apiBase}/articles/preview`, async ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const body = (await request.json()) as Schemas["ArticlePreviewRequest"];
    const { content, images } = renderedBody(body.text);
    const summary = body.summary?.trim() ?? "";
    return HttpResponse.json({
      content,
      summary: summary === "" ? body.text.slice(0, 200) : summary,
      summary_from_body: summary === "",
      bytes: new TextEncoder().encode(content).length,
      images,
      ...limits,
    } satisfies Schemas["ArticlePreview"]);
  }),
  // operationId editArticle.
  http.put(`${apiBase}/articles/:id`, async ({ request }) => {
    if (!authorized(request)) return unauthenticated(request);
    const body = (await request.json()) as Schemas["EditArticleRequest"];
    return HttpResponse.json(articleStatus({ title: body.title, summary: body.summary ?? "" }, body.text));
  }),
  // operationId createArticle: the editor publishes through the drafts, but the operation has its handler.
  http.post(`${apiBase}/articles`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json(articleStatus({ title: "An article", summary: "" }, "The body"), { status: 201 }),
    ),
  ),
];

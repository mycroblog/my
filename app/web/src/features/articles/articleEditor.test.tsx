// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (web-client/screens/article-editor.md) through the whole app, with MSW for the server: the
// preview is the server's rendering of the body, an image dropped into the body uploads and shows in the preview, the
// autosave keeps the text through a conflict with another device's save, and publishing publishes the saved draft.
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, test } from "vitest";
import { draftFixture, mediaFixture, type Schemas, viewerFixture } from "../../test/fixtures";
import { nodeUploadFile } from "../../test/multipart";
import { apiBase, authorized, signedInHandler, unauthenticated, withViewer } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { articleId, articleStatus } from "./test/handlers";

/** The ID the server gives the image the tests upload. */
const uploadedMediaId = "0192f0c4-7a8e-7000-8000-0000000005b1";

/** A draft's answer with its ETag. */
function draftResponse(draft: Schemas["Draft"], etag: string, status = 200) {
  return HttpResponse.json(draft, { status, headers: { ETag: etag } });
}

/** An article's draft `id` with `title` and the body `text`. */
function articleDraft(id: string, title: string, text: string): Schemas["Draft"] {
  return { ...draftFixture(id, { text }), article: { title, summary: "", tags: ["garden"], images: [] } };
}

/**
 * The drafts on the server, in memory: a save sends If-Match, and a save over another version is a 412. Returns the
 * writes, and the bodies of the publishes.
 */
function draftsServer(initial: { draft: Schemas["Draft"]; etag: string }[] = []) {
  const drafts = new Map(initial.map((d) => [d.draft.id, d]));
  const writes: string[] = [];
  let next = 0;
  server.use(
    http.get(`${apiBase}/drafts/:id`, ({ request, params }) =>
      withViewer(request, () => {
        const saved = drafts.get(String(params["id"]));
        return saved === undefined
          ? HttpResponse.json({ type: "about:blank", title: "Not found", status: 404 }, { status: 404 })
          : draftResponse(saved.draft, saved.etag);
      }),
    ),
    http.post(`${apiBase}/drafts`, async ({ request }) => {
      if (!authorized(request)) return unauthenticated(request);
      const body = (await request.json()) as Schemas["DraftRequest"];
      next += 1;
      const id = `new${String(next)}`;
      writes.push(`POST ${body.article?.title ?? ""}|${body.text ?? ""}`);
      const draft = articleDraft(id, body.article?.title ?? "", body.text ?? "");
      drafts.set(id, { draft, etag: '"v1"' });
      return draftResponse(draft, '"v1"', 201);
    }),
    http.put(`${apiBase}/drafts/:id`, async ({ request, params }) => {
      if (!authorized(request)) return unauthenticated(request);
      const id = String(params["id"]);
      const body = (await request.json()) as Schemas["DraftRequest"];
      const ifMatch = request.headers.get("If-Match") ?? "";
      writes.push(`PUT ${id} ${ifMatch} ${body.article?.title ?? ""}|${body.text ?? ""}`);
      const saved = drafts.get(id);
      if (saved === undefined || saved.etag !== ifMatch) {
        return HttpResponse.json({ type: "about:blank", title: "Precondition Failed", status: 412 }, { status: 412 });
      }
      const etag = `"v${String(Number(ifMatch.replaceAll('"', "").slice(1)) + 1)}"`;
      const draft = articleDraft(id, body.article?.title ?? "", body.text ?? "");
      drafts.set(id, { draft, etag });
      return draftResponse(draft, etag);
    }),
    http.post(`${apiBase}/drafts/:id/publish`, ({ request, params }) =>
      withViewer(request, () => {
        const id = String(params["id"]);
        writes.push(`PUBLISH ${id}`);
        const saved = drafts.get(id);
        drafts.delete(id);
        const title = saved?.draft.article?.title ?? "";
        return HttpResponse.json(articleStatus({ title, summary: "" }, saved?.draft.text ?? ""), { status: 201 });
      }),
    ),
  );
  return writes;
}

function body(): HTMLTextAreaElement {
  const element = screen.getByRole("textbox", { name: "Body" });
  if (!(element instanceof HTMLTextAreaElement)) throw new Error("the body is no text area");
  return element;
}

function preview() {
  return screen.getByRole("region", { name: "Preview" });
}

describe("the article editor", () => {
  test("previews the server's rendering, autosaves the draft, and publishes it", async () => {
    server.use(signedInHandler);
    const writes = draftsServer();
    const { user, router } = await renderApp({ path: "/articles/new" });

    expect(await screen.findByRole("heading", { level: 1, name: "New article" })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Title" }), "Tomatoes");
    await user.type(body(), "Grow them in deep pots.");
    // The bold button makes the selection bold, as the server then renders it.
    body().setSelectionRange(5, 9);
    await user.click(screen.getByRole("button", { name: "Bold" }));
    await waitFor(() => {
      expect(body()).toHaveValue("Grow **them** in deep pots.");
    });

    expect(await within(preview()).findByText("them", { selector: "strong" })).toBeInTheDocument();
    expect(within(preview()).getByRole("heading", { level: 3, name: "Tomatoes" })).toBeInTheDocument();
    expect(await screen.findByText("Draft saved")).toBeInTheDocument();
    expect(writes).toEqual(["POST Tomatoes|Grow **them** in deep pots."]);

    await user.click(screen.getByRole("button", { name: "Publish" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/@${viewerFixture.acct}/status/${articleId}`);
    });
    // The last change is already saved, so publishing saves the same text over the version it read and publishes it.
    expect(writes).toEqual([
      "POST Tomatoes|Grow **them** in deep pots.",
      'PUT new1 "v1" Tomatoes|Grow **them** in deep pots.',
      "PUBLISH new1",
    ]);
  });

  test("says what is missing instead of publishing", async () => {
    server.use(signedInHandler);
    const writes = draftsServer();
    const { user } = await renderApp({ path: "/articles/new" });
    await user.click(await screen.findByRole("button", { name: "Publish" }));

    const title = screen.getByRole("textbox", { name: "Title" });
    expect(title).toHaveAccessibleDescription("Give the article a title.");
    expect(writes).toEqual([]);
  });

  test("uploads an image dropped into the body, and the preview shows it once it is processed", async () => {
    server.use(signedInHandler);
    draftsServer();
    server.use(
      http.post(`${apiBase}/media`, () =>
        HttpResponse.json(mediaFixture(uploadedMediaId, "processing"), { status: 202 }),
      ),
    );
    const file = nodeUploadFile("GIF89a", "tomato.gif", "image/gif");
    const { user } = await renderApp({ path: "/articles/new" });
    await user.type(await screen.findByRole("textbox", { name: "Body" }), "Before the picture");

    fireEvent.drop(body(), { dataTransfer: { types: ["Files"], files: [file] } });

    await waitFor(() => {
      expect(body()).toHaveValue(`Before the picture\n\n![](media:${uploadedMediaId})`);
    });
    const image = await within(preview()).findByRole("presentation");
    expect(image).toHaveAttribute("src", mediaFixture(uploadedMediaId).variants?.large.url);
    // The draft with the image is saved, so nothing is left to save when the editor closes after the test.
    expect(await screen.findByText("Draft saved")).toBeInTheDocument();
  });

  test("keeps the text when another device saved the draft meanwhile, and saves it as a new draft", async () => {
    server.use(signedInHandler);
    const writes = draftsServer([{ draft: articleDraft("d1", "Tomatoes", "From this device"), etag: '"v3"' }]);
    // The editor read the draft before another device's save: its version is older than the server's.
    server.use(
      http.get(`${apiBase}/drafts/d1`, ({ request }) =>
        withViewer(request, () => draftResponse(articleDraft("d1", "Tomatoes", "From this device"), '"v2"')),
      ),
    );
    const { user } = await renderApp({ path: "/articles/new?draft=d1" });
    expect(await screen.findByRole("textbox", { name: "Body" })).toHaveValue("From this device");
    expect(screen.getByRole("textbox", { name: "Hashtags" })).toHaveValue("#garden");

    await user.type(body(), ", and more");

    // The page has other alerts (the route announcer's); the conflict is the one that says it.
    const alert = await waitFor(() => {
      const found = screen
        .getAllByRole("alert")
        .find((element) => element.textContent.includes("This draft was changed on another device"));
      if (found === undefined) throw new Error("no alert of the conflict");
      return found;
    });
    expect(body()).toHaveValue("From this device, and more");
    expect(writes).toEqual(['PUT d1 "v2" Tomatoes|From this device, and more']);

    await user.click(within(alert).getByRole("button", { name: "Save as a new draft" }));
    expect(await screen.findByText("Draft saved")).toBeInTheDocument();
    expect(writes).toEqual([
      'PUT d1 "v2" Tomatoes|From this device, and more',
      "POST Tomatoes|From this device, and more",
    ]);
    expect(body()).toHaveValue("From this device, and more");
  });

  test("opens an article's draft from the drafts page", async () => {
    server.use(signedInHandler);
    const draft = articleDraft("d1", "Tomatoes", "From this device");
    draftsServer([{ draft, etag: '"v1"' }]);
    server.use(
      http.get(`${apiBase}/drafts`, ({ request }) =>
        withViewer(request, () => {
          const scheduled = new URL(request.url).searchParams.get("scheduled") === "true";
          return HttpResponse.json({ items: scheduled ? [] : [draft] } satisfies Schemas["DraftPage"]);
        }),
      ),
    );
    const { user } = await renderApp({ path: "/drafts" });

    await user.click(await screen.findByRole("button", { name: "Edit: Tomatoes" }));

    expect(await screen.findByRole("heading", { level: 1, name: "New article" })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "Title" })).toHaveValue("Tomatoes");
  });

  test("edits a published article in place", async () => {
    server.use(signedInHandler);
    const edits: Schemas["EditArticleRequest"][] = [];
    server.use(
      http.get(`${apiBase}/statuses/${articleId}`, () =>
        HttpResponse.json(articleStatus({ title: "Tomatoes", summary: "" }, "The body")),
      ),
      http.get(`${apiBase}/statuses/${articleId}/source`, ({ request }) =>
        withViewer(request, () =>
          HttpResponse.json({
            id: articleId,
            text: "The body",
            spoiler_text: "",
            language: "en",
            article: { title: "Tomatoes", summary: "", tags: ["garden"] },
          } satisfies Schemas["StatusSource"]),
        ),
      ),
      http.put(`${apiBase}/articles/${articleId}`, async ({ request }) => {
        const edit = (await request.json()) as Schemas["EditArticleRequest"];
        edits.push(edit);
        return HttpResponse.json(articleStatus({ title: edit.title, summary: "" }, edit.text));
      }),
    );
    const { user, router } = await renderApp({ path: `/articles/${articleId}/edit` });

    expect(await screen.findByRole("heading", { level: 1, name: "Edit article" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Schedule" })).not.toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "Title" }), " on a balcony");
    await user.click(screen.getByRole("button", { name: "Save changes" }));

    await waitFor(() => {
      expect(router.state.location.pathname).toBe(`/@${viewerFixture.acct}/status/${articleId}`);
    });
    expect(edits).toEqual([
      {
        title: "Tomatoes on a balcony",
        summary: "",
        text: "The body",
        tags: ["garden"],
        language: "en",
        sensitive: false,
      },
    ]);
  });
});

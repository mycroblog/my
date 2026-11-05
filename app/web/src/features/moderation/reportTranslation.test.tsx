// SPDX-License-Identifier: AGPL-3.0-only
// Translating a report's posts and its comment through the whole app (web-client/README.md T4, ADR-0148 §13): a
// moderator with manage_reports, MSW for the admin API, and the instance's translation settings that decide what the
// page offers.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { instanceFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, operatorHandler, signedInHandler } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import { reportFixture } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000b1";
const comment = "Il publie le même lien encore et encore.";
const report = reportFixture({
  comment,
  statuses: [{ status: { ...statusFixture(postId), content: "<p>Bonjour tout le monde</p>", language: "fr" } }],
});

const provider = { id: "fake", name: "Fake Translate", url: "https://translate.example/" };

type Visibility = Schemas["Visibility"];

let instanceReads = 0;

/**
 * Serves the report, an instance whose provider takes `visibilities` and that lets reports go to it or not, and records
 * what the report's translation operations receive.
 */
function serve({
  reportsToServer,
  visibilities,
  browser = false,
}: {
  reportsToServer: boolean;
  visibilities: Visibility[];
  browser?: boolean;
}) {
  const requests: string[] = [];
  server.use(
    http.get(`${apiBase}/instance`, () => {
      instanceReads += 1;
      return HttpResponse.json({
        ...instanceFixture,
        translation: {
          browser,
          providers: [{ ...provider, visibilities }],
          reports_to_server: reportsToServer,
        },
      });
    }),
    http.get(`${apiBase}/admin/reports/:id`, () => HttpResponse.json(report)),
    http.get(`${apiBase}/admin/accounts/:id/actions`, () => HttpResponse.json({ items: [] })),
    http.post(`${apiBase}/admin/reports/:id/statuses/:statusId/translation`, ({ params }) => {
      requests.push(`status ${String(params["statusId"])}`);
      return HttpResponse.json({
        language: "en",
        source_language: "fr",
        content: "<p>Hello everyone</p>",
        spoiler_text: "",
        media_attachments: [],
        poll_options: [],
        provider,
      } satisfies Schemas["StatusTranslation"]);
    }),
    http.post(`${apiBase}/admin/reports/:id/comment/translation`, () => {
      requests.push("comment");
      return HttpResponse.json({
        language: "en",
        source_language: "fr",
        comment: "He posts the same link again and again.",
        provider,
      } satisfies Schemas["ReportCommentTranslation"]);
    }),
  );
  return requests;
}

/** Waits until the page read GET /instance, which decides the actions, and could have drawn what it says. */
async function openedReport() {
  const { user } = await renderApp({ path: `/moderation/reports/${report.id}` });
  await screen.findByRole("heading", { level: 1, name: "Report" });
  await waitFor(() => {
    expect(instanceReads).toBeGreaterThan(0);
  });
  await delay(50);
  return user;
}

function postsSection() {
  return screen.getByRole("region", { name: "Reported posts" });
}

function summarySection() {
  return screen.getByRole("region", { name: "Report" });
}

afterEach(() => {
  vi.unstubAllGlobals();
});

beforeEach(() => {
  instanceReads = 0;
  server.use(signedInHandler, operatorHandler(["manage_reports"]));
  document.documentElement.lang = "en";
  document.documentElement.dir = "ltr";
});

describe("translating a report", () => {
  test("translates a reported post and the comment through the report", async () => {
    const requests = serve({ reportsToServer: true, visibilities: ["public", "private"] });
    const user = await openedReport();

    const posts = postsSection();
    await user.click(await within(posts).findByRole("button", { name: "Translate" }));
    expect(await within(posts).findByText("Hello everyone")).toBeInTheDocument();
    expect(within(posts).getByText(/Fake Translate/)).toBeInTheDocument();

    const summary = summarySection();
    await user.click(within(summary).getByRole("button", { name: "Translate" }));
    expect(await within(summary).findByText("He posts the same link again and again.")).toHaveAttribute("lang", "en");
    expect(within(summary).queryByText(comment)).not.toBeInTheDocument();
    expect(requests).toEqual([`status ${postId}`, "comment"]);

    // Back to the reporter's words, with the same button, and again without asking the server.
    await user.click(within(summary).getByRole("button", { name: "Show original" }));
    expect(within(summary).getByText(comment)).toBeInTheDocument();
    await user.click(within(summary).getByRole("button", { name: "Translate" }));
    expect(await within(summary).findByText("He posts the same link again and again.")).toBeInTheDocument();
    expect(requests).toHaveLength(2);
  });

  test("offers nothing while the operator keeps reports from the providers", async () => {
    serve({ reportsToServer: false, visibilities: ["public", "local", "unlisted", "private", "direct"] });
    await openedReport();
    expect(within(postsSection()).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
    expect(within(summarySection()).getByText(comment)).toBeInTheDocument();
    expect(within(summarySection()).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test("offers the comment only to a provider allowed followers-only posts", async () => {
    serve({ reportsToServer: true, visibilities: ["public", "local", "unlisted"] });
    await openedReport();
    expect(await within(postsSection()).findByRole("button", { name: "Translate" })).toBeInTheDocument();
    expect(within(summarySection()).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test("translates in the browser while the operator keeps reports from the providers", async () => {
    // The browser's APIs, as a stand-in: the detector finds French, and the translator marks what it translates.
    vi.stubGlobal("LanguageDetector", {
      availability: () => Promise.resolve("available"),
      create: () =>
        Promise.resolve({
          detect: () => Promise.resolve([{ detectedLanguage: "fr", confidence: 0.9 }]),
          destroy: () => undefined,
        }),
    });
    vi.stubGlobal("Translator", {
      availability: () => Promise.resolve("available"),
      create: () =>
        Promise.resolve({ translate: (input: string) => Promise.resolve(`EN ${input}`), destroy: () => undefined }),
    });
    const requests = serve({ reportsToServer: false, visibilities: ["public", "private"], browser: true });
    const user = await openedReport();

    const posts = postsSection();
    await user.click(await within(posts).findByRole("button", { name: "Translate" }));
    expect(await within(posts).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
    const summary = summarySection();
    await user.click(await within(summary).findByRole("button", { name: "Translate" }));
    expect(await within(summary).findByText(`EN ${comment}`)).toHaveAttribute("lang", "en");
    expect(requests).toEqual([]);
  });
});

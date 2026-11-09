// SPDX-License-Identifier: AGPL-3.0-only
// Translating a post through the whole app (web-client/README.md T4, FR-120, ADR-0148): the real router and
// QueryClient, MSW for the client API with what the fake provider would answer.
import { screen, waitFor, within } from "@testing-library/react";
import { delay, http, HttpResponse } from "msw";
import { beforeEach, describe, expect, test } from "vitest";
import { instanceFixture, mediaFixture, type Schemas, statusFixture, viewerStatusFixture } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status, Visibility } from "./api/status";
import { isInLanguage, statusTranslationQueryOptions } from "./api/translation";
import { pollFixture } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000d0";
const original = "Bonjour tout le monde";
const translated = "Hello everyone";

/** A post of Bob in French, and its page. */
const french: Status = { ...statusFixture(postId), content: `<p>${original}</p>`, language: "fr" };

const fake: Schemas["InstanceTranslationProvider"] = {
  id: "fake",
  name: "Fake Translate",
  url: "https://translate.example/",
  visibilities: ["public", "local", "unlisted"],
};

const answer: Schemas["StatusTranslation"] = {
  language: "en",
  source_language: "fr",
  content: `<p>${translated}</p>`,
  spoiler_text: "",
  media_attachments: [],
  poll_options: [],
  provider: { id: "fake", name: "Fake Translate", url: "https://translate.example/" },
};

function pathOf(status: Status): string {
  return `/@${status.account.acct}/status/${status.id}`;
}

function signedIn() {
  server.use(http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")));
}

let instanceReads = 0;

/** Waits until the page read GET /instance, which decides the action, and could have drawn what it says. */
async function instanceRead() {
  await waitFor(() => {
    expect(instanceReads).toBeGreaterThan(0);
  });
  await delay(50);
}

/** Serves `status`, the instance with `providers`, and records what POST /statuses/{id}/translation receives. */
function serve(
  status: Status,
  {
    providers = [fake],
    respond = () => HttpResponse.json(answer),
  }: {
    providers?: Schemas["InstanceTranslationProvider"][];
    respond?: () => Response | Promise<Response>;
  } = {},
) {
  const requests: { id: string; body: Schemas["StatusTranslationRequest"] }[] = [];
  server.use(
    http.get(`${apiBase}/instance`, () => {
      instanceReads += 1;
      return HttpResponse.json({ ...instanceFixture, translation: { browser: false, providers } });
    }),
    http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(status)),
    http.post(`${apiBase}/statuses/:id/translation`, async ({ params, request }) => {
      requests.push({
        id: String(params["id"]),
        body: (await request.json()) as Schemas["StatusTranslationRequest"],
      });
      return respond();
    }),
  );
  return requests;
}

async function openedPost() {
  return screen.findByRole("article", { name: /^Opened post/ });
}

/** The element with `dir="auto"` that holds exactly `text`: user content in its own direction (Y10). */
function findUserText(scope: HTMLElement, text: string) {
  return within(scope).findByText(
    (_, element) => element?.getAttribute("dir") === "auto" && element.textContent === text,
  );
}

/** The announcer's polite live region; the app's offline bar is another status region, empty while online. */
function announcer() {
  const region = screen.getAllByRole("status").find((element) => element.hasAttribute("data-live-announcer"));
  if (region === undefined) throw new Error("the announcer is not rendered");
  return region;
}

beforeEach(() => {
  instanceReads = 0;
  sessionStorage.clear();
  localStorage.clear();
});

describe("the action", () => {
  test.each<[string, Visibility, Schemas["InstanceTranslationProvider"][]]>([
    ["no provider is enabled", "public", []],
    ["no provider may receive a followers-only post", "private", [fake]],
    ["no provider may receive a direct post", "direct", [fake]],
  ])("is not there when %s", async (_, visibility, providers) => {
    signedIn();
    serve({ ...french, visibility }, { providers });
    await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await within(post).findByRole("button", { name: "Favourite" });
    await instanceRead();
    expect(within(post).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test.each<[string, string]>([
    ["the UI language", "en"],
    ["a regional form of the UI language", "en-GB"],
  ])("is not there on a post in %s", async (_, language) => {
    signedIn();
    serve({ ...french, language });
    await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await within(post).findByRole("button", { name: "Favourite" });
    await instanceRead();
    expect(within(post).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test("is not there for a visitor, whom the server does not translate for", async () => {
    serve(french);
    await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await within(post).findByText(original);
    await instanceRead();
    expect(within(post).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
  });

  test("is there on a followers-only post when a provider may receive it, and on a post of no known language, and sends the UI language", async () => {
    signedIn();
    const requests = serve(
      { ...french, visibility: "private", language: "und" },
      { providers: [{ ...fake, visibilities: ["private"] }] },
    );
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByText(translated);
    expect(requests).toEqual([{ id: postId, body: { target_language: "en" } }]);
  });

  test("translates the boosted post of a boost", async () => {
    signedIn();
    const boost: Status = viewerStatusFixture({ id: "0192f0c4-7a8e-7000-8000-0000000000d1", boost: french });
    const requests = serve(boost);
    const { user } = await renderApp({ path: pathOf(boost) });

    await user.click(await screen.findByRole("button", { name: "Translate" }));
    await screen.findByText(translated);
    expect(requests[0]?.id).toBe(postId);
  });
});

describe("the translation", () => {
  test("replaces the text in place with a line after it that says from which language and by whom, keeps the focus, says so, and goes back to the original without another request", async () => {
    signedIn();
    const requests = serve(french);
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    const toggle = await within(post).findByRole("button", { name: "Translate" });
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    await user.click(toggle);

    // The body says its own language and direction.
    const body = await findUserText(post, translated);
    expect(body).toHaveAttribute("lang", "en");
    expect(within(post).queryByText(original)).not.toBeInTheDocument();

    // The line after the text says what happened and by whom, in the article's description, with a link to the
    // provider and the way back.
    const link = within(post).getByRole("link", { name: /Fake Translate/ });
    expect(link).toHaveAttribute("href", "https://translate.example/");
    expect(post).toHaveAccessibleDescription(expect.stringContaining("Translated from French Fake Translate"));
    expect(body.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    await waitFor(() => {
      expect(announcer()).toHaveTextContent("Machine translated by Fake Translate");
    });

    // The toggle keeps the focus and its name, and is pressed while the translation shows.
    expect(toggle).toHaveFocus();
    expect(toggle).toHaveAccessibleName("Translate");
    expect(toggle).toHaveAttribute("aria-pressed", "true");

    // The line's way back goes to the original, and the focus to the toggle, since the line goes away.
    await user.click(within(post).getByRole("button", { name: "Show original" }));
    expect(await within(post).findByText(original)).toBeVisible();
    expect(within(post).queryByText(/Translated from/)).not.toBeInTheDocument();
    expect(toggle).toHaveFocus();
    expect(toggle).toHaveAttribute("aria-pressed", "false");
    await waitFor(() => {
      expect(announcer()).toHaveTextContent("Showing the original post.");
    });

    // Asking again shows what the page already has, and the toggle goes back too.
    await user.click(toggle);
    await within(post).findByText(translated);
    expect(requests).toHaveLength(1);
    await user.click(toggle);
    expect(await within(post).findByText(original)).toBeVisible();
  });

  test("is in the menu of more actions too, where a narrow card keeps it, and the focus goes back to the menu's button", async () => {
    signedIn();
    serve(french);
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    const more = await within(post).findByRole("button", { name: "More actions" });
    await user.click(more);
    await user.click(await screen.findByRole("menuitem", { name: "Translate" }));
    await findUserText(post, translated);

    await user.click(more);
    expect(await screen.findByRole("menuitem", { name: "Show original" })).toBeVisible();
    await user.keyboard("{Escape}");

    // In a narrow card the toggle is not there to take the focus back from the line.
    const toggle = within(post).getByRole("button", { name: "Translate" });
    // jsdom has no layout: say what a browser would say of a toggle that `display: none` hides.
    Object.defineProperty(toggle, "checkVisibility", { value: () => false });
    await user.click(within(post).getByRole("button", { name: "Show original" }));
    expect(await within(post).findByText(original)).toBeVisible();
    expect(more).toHaveFocus();
  });

  test("does not call the author's own words a machine translation", async () => {
    signedIn();
    const { provider: _, ...own } = answer;
    serve(french, { respond: () => HttpResponse.json(own) });
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByText(translated);
    expect(within(post).getByText("Not a machine translation: this is the author's own text.")).toBeVisible();
    expect(within(post).queryByText(/Machine translated/)).not.toBeInTheDocument();
  });

  test("names a provider without an address in words, with its logo beside them", async () => {
    signedIn();
    serve(french, {
      respond: () =>
        HttpResponse.json({
          ...answer,
          provider: { id: "fake", name: "Fake Translate", logo_url: `https://${instanceFixture.domain}/logo.svg` },
        }),
    });
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await findUserText(post, translated);
    expect(post).toHaveAccessibleDescription(
      expect.stringContaining("Translated from French Fake Translate Show original"),
    );
    expect(within(post).queryByRole("link", { name: /Fake Translate/ })).not.toBeInTheDocument();
    // Decorative: the sentence names what it shows.
    const logos = within(post)
      .getAllByAltText("")
      .filter((image) => image.getAttribute("src") === `https://${instanceFixture.domain}/logo.svg`);
    expect(logos).toHaveLength(1);
  });

  test("translates the content warning and the poll's options, and goes back from both", async () => {
    signedIn();
    const warned: Status = {
      ...french,
      spoiler_text: "Attention",
      poll: pollFixture,
    };
    serve(warned, {
      respond: () =>
        HttpResponse.json({
          ...answer,
          spoiler_text: "Careful",
          poll_options: ["Rouge", "Bleu", "Vert"],
        }),
    });
    const { user } = await renderApp({ path: pathOf(warned) });

    const post = await openedPost();
    expect(within(post).getByText("Attention")).toBeVisible();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await findUserText(post, "Careful")).toHaveAttribute("lang", "en");
    // The line that says what this is stays outside the closed warning.
    expect(within(post).getByText(/Translated from French/)).toBeVisible();

    await user.click(within(post).getByRole("button", { name: "Show content" }));
    expect(within(post).getByText("Bleu")).toBeVisible();
    await user.click(within(post).getByRole("button", { name: "Show original" }));
    expect(within(post).getByText("Blue")).toBeVisible();
    expect(within(post).getByText("Attention")).toBeVisible();
  });

  test("translates the alt text of the media that has any, by the media's ID", async () => {
    signedIn();
    const pictured: Status = {
      ...french,
      media_attachments: [
        mediaFixture("0192f0c4-7a8e-7000-8000-0000000000d3", "ready", "Un chat"),
        mediaFixture("0192f0c4-7a8e-7000-8000-0000000000d4", "ready", "Un chien"),
      ],
    };
    serve(pictured, {
      respond: () =>
        HttpResponse.json({
          ...answer,
          media_attachments: [{ id: "0192f0c4-7a8e-7000-8000-0000000000d4", description: "A dog" }],
        }),
    });
    const { user } = await renderApp({ path: pathOf(pictured) });

    const post = await openedPost();
    await within(post).findByRole("button", { name: /Un chat/ });
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByRole("button", { name: /A dog/ });
    expect(within(post).getByRole("button", { name: /Un chat/ })).toBeVisible();
  });

  test("keeps the warning of the post when the answer has none", async () => {
    signedIn();
    const warned: Status = { ...french, spoiler_text: "Attention" };
    serve(warned);
    const { user } = await renderApp({ path: pathOf(warned) });

    const post = await openedPost();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByRole("button", { name: "Show original" });
    expect(within(post).getByText("Attention")).toBeVisible();
  });

  test("stays in memory: nothing of it reaches the browser's storage", async () => {
    signedIn();
    serve(french);
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByText(translated);

    for (const storage of [localStorage, sessionStorage]) {
      for (const key of Object.keys(storage)) {
        expect(storage.getItem(key)).not.toContain(translated);
      }
    }
  });
});

describe("a failure", () => {
  test.each<[string, () => Response, string]>([
    [
      "the monthly limit",
      () => problem(429, "/api/v1/problems/translation-limit", "Too many"),
      "The monthly translation limit has been reached, so this post wasn't translated. You can translate again next month.",
    ],
    [
      "a post no provider may receive",
      () => problem(422, "/api/v1/problems/translation-unavailable", "Unavailable"),
      "This server can't translate this post.",
    ],
    [
      "a language no provider translates",
      () => problem(422, "/api/v1/problems/translation-unsupported-language", "Unsupported"),
      "This server's translation service doesn't support this language.",
    ],
    [
      "a provider without a usable answer",
      () => problem(503, "/api/v1/problems/translation-failed", "Failed"),
      "This post couldn't be translated right now. Try again in a moment.",
    ],
    ["anything else", () => problem(500, "about:blank", "It broke"), "This post couldn't be translated: It broke"],
  ])("of %s is said in place, with the original readable and the focus on the button", async (_, respond, words) => {
    signedIn();
    serve(french, { respond });
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    const button = await within(post).findByRole("button", { name: "Translate" });
    await user.click(button);

    const alert = await within(post).findByRole("alert");
    expect(alert).toHaveTextContent(words);
    expect(within(post).getByText(original)).toBeVisible();
    expect(button).toHaveFocus();
    expect(button).toHaveAccessibleName("Translate");
    // A limit is this button's failure, not the rate limit of the whole app.
    expect(screen.queryByText(/^Too many requests/)).not.toBeInTheDocument();
  });

  test("goes away when another try succeeds", async () => {
    signedIn();
    let failing = true;
    serve(french, {
      respond: () =>
        failing ? problem(503, "/api/v1/problems/translation-failed", "Failed") : HttpResponse.json(answer),
    });
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    const button = await within(post).findByRole("button", { name: "Translate" });
    await user.click(button);
    await within(post).findByRole("alert");
    failing = false;
    await user.click(button);

    await within(post).findByText(translated);
    expect(within(post).queryByRole("alert")).not.toBeInTheDocument();
  });

  test("while it is on its way, the line says so, and the toggle keeps its focus and takes no second press", async () => {
    signedIn();
    const requests = serve(french, {
      respond: async () => {
        await delay(100);
        return HttpResponse.json(answer);
      },
    });
    const { user } = await renderApp({ path: pathOf(french) });

    const post = await openedPost();
    const toggle = await within(post).findByRole("button", { name: "Translate" });
    await user.click(toggle);
    expect(within(post).getByText("Translating…")).toBeVisible();
    expect(toggle).toHaveFocus();
    await user.click(toggle);

    await within(post).findByText(translated);
    expect(requests).toHaveLength(1);
  });
});

describe("the cache of a translation", () => {
  test("has a place for each version of a post and each language", () => {
    const key = (post: { id: string; edited_at?: string }, language: string) =>
      statusTranslationQueryOptions(post, language).queryKey;
    expect(key({ id: "1" }, "en")).toEqual(key({ id: "1" }, "en"));
    expect(key({ id: "1", edited_at: "2026-10-16T12:00:00Z" }, "en")).not.toEqual(key({ id: "1" }, "en"));
    expect(key({ id: "1" }, "ja")).not.toEqual(key({ id: "1" }, "en"));
  });
});

describe("isInLanguage", () => {
  test.each<[string, string, boolean]>([
    ["en", "en", true],
    ["EN", "en", true],
    ["en-GB", "en", true],
    ["pt-BR", "pt-PT", true],
    ["fr", "en", false],
    ["und", "en", false],
    ["zh-Hant", "zh-Hans", false],
    ["zh-Hant", "zh", false],
    ["zh", "zh-Hant", false],
    ["zh-Hant", "zh-Hant", true],
  ])("%s in %s: %s", (language, target, expected) => {
    expect(isInLanguage(language, target)).toBe(expected);
  });
});

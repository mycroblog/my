// SPDX-License-Identifier: AGPL-3.0-only
// Translating a post in the browser through the whole app (web-client/README.md T4, FR-120, ADR-0148 §12): the real
// router and QueryClient, MSW for the client API, and a stand-in for the browser's Translator and Language Detector
// APIs, which behaves as Chrome 156 did when it was tried.
import { screen, waitFor, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { instanceFixture, type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, oauthPaths, problem, tokenResponse } from "../../test/msw/http";
import { server } from "../../test/msw/server";
import { renderApp } from "../../test/render";
import type { Status, Visibility } from "./api/status";
import { markersIntact } from "./api/browserTranslation";
import { pollFixture } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000e0";
const french: Status = { ...statusFixture(postId), content: "<p>Bonjour tout le monde</p>", language: "fr" };

type Availability = "available" | "downloadable" | "downloading" | "unavailable";
type Monitor = { addEventListener: (type: string, listener: (event: { loaded: number }) => void) => void };

/** The stand-in for the APIs: what each answers, and what each was given. */
function browserApis({
  availability = "available",
  answer = (input: string) => `EN ${input}`,
  create,
  progress = [],
}: {
  availability?: Availability;
  answer?: (input: string) => string;
  create?: () => Promise<void>;
  progress?: number[];
} = {}) {
  const inputs: string[] = [];
  const pairs: unknown[] = [];
  const destroyed = vi.fn();
  vi.stubGlobal("Translator", {
    availability: vi.fn(() => Promise.resolve(availability)),
    create: vi.fn(
      async (options: { sourceLanguage: string; targetLanguage: string; monitor?: (monitor: Monitor) => void }) => {
        pairs.push([options.sourceLanguage, options.targetLanguage]);
        await create?.();
        options.monitor?.({
          addEventListener: (_, listener) => {
            for (const loaded of progress) listener({ loaded });
          },
        });
        return {
          translate: (input: string) => {
            inputs.push(input);
            return Promise.resolve(answer(input));
          },
          destroy: destroyed,
        };
      },
    ),
  });
  return { inputs, pairs, destroyed };
}

function detectorApi(results: { detectedLanguage: string; confidence: number }[], availability: Availability) {
  vi.stubGlobal("LanguageDetector", {
    availability: () => Promise.resolve(availability),
    create: () => Promise.resolve({ detect: () => Promise.resolve(results), destroy: () => undefined }),
  });
}

function brands(...names: string[]) {
  Object.defineProperty(navigator, "userAgentData", {
    configurable: true,
    value: { brands: names.map((brand) => ({ brand })) },
  });
}

/** A provider that receives the posts that a provider receives by default. */
const provider: Schemas["InstanceTranslationProvider"] = {
  id: "fake",
  name: "Fake",
  visibilities: ["public", "local", "unlisted"],
};

/** What the provider answers for `french`. */
const providerAnswer: Schemas["StatusTranslation"] = {
  language: "en",
  source_language: "fr",
  content: "<p>Hello everyone</p>",
  spoiler_text: "",
  media_attachments: [],
  poll_options: [],
  provider: { id: "fake", name: "Fake" },
};

/** Serves `status` and the instance, and counts what reaches POST /statuses/{id}/translation, which `respond` answers. */
function serve(
  status: Status,
  {
    browser = true,
    providers = [],
    respond = () => HttpResponse.json({}, { status: 500 }),
  }: {
    browser?: boolean;
    providers?: Schemas["InstanceTranslationProvider"][];
    respond?: () => Response;
  } = {},
) {
  const requests: unknown[] = [];
  server.use(
    http.post(oauthPaths.refresh, () => tokenResponse("test-access-token")),
    http.get(`${apiBase}/instance`, () =>
      HttpResponse.json({ ...instanceFixture, translation: { browser, providers } }),
    ),
    http.get(`${apiBase}/statuses/:id`, () => HttpResponse.json(status)),
    http.post(`${apiBase}/statuses/:id/translation`, async ({ request }) => {
      requests.push(await request.text());
      return respond();
    }),
  );
  return requests;
}

async function openedPost(status: Status = french) {
  await renderApp({ path: `/@${status.account.acct}/status/${status.id}` });
  return screen.findByRole("article", { name: /^Opened post/ });
}

async function expectNoAction(post: HTMLElement) {
  await within(post).findByRole("button", { name: "Favourite" });
  // Long enough for the instance and the availability to have answered, had they changed anything.
  await new Promise((resolve) => setTimeout(resolve, 100));
  expect(within(post).queryByRole("button", { name: "Translate" })).not.toBeInTheDocument();
}

beforeEach(() => {
  sessionStorage.clear();
  localStorage.clear();
});

afterEach(() => {
  vi.unstubAllGlobals();
  Reflect.deleteProperty(navigator, "userAgentData");
});

describe("the action", () => {
  test("is not there when the operator did not enable it, even in a browser that has the API", async () => {
    browserApis();
    serve(french, { browser: false });
    await expectNoAction(await openedPost());
  });

  test("is not there in a browser without the API, and there is no way at all on a server with no provider", async () => {
    serve(french);
    await expectNoAction(await openedPost());
  });

  test("is not there for a pair of languages that the browser reports unavailable", async () => {
    browserApis({ availability: "unavailable" });
    serve(french);
    await expectNoAction(await openedPost());
  });

  test("is not there for a language tag that the browser refuses", async () => {
    browserApis();
    vi.stubGlobal("Translator", { availability: () => Promise.reject(new RangeError("Invalid language tag")) });
    serve(french);
    await expectNoAction(await openedPost());
  });

  test("is not there on a post of no known language when the browser has no detector", async () => {
    browserApis();
    serve({ ...french, language: "und" });
    await expectNoAction(await openedPost({ ...french, language: "und" }));
  });

  test("is not there on a post of no known language when the detector is unavailable", async () => {
    browserApis();
    detectorApi([], "unavailable");
    serve({ ...french, language: "und" });
    await expectNoAction(await openedPost({ ...french, language: "und" }));
  });

  test.each<Visibility>(["public", "local", "unlisted", "private", "direct"])(
    "translates a post of visibility %s and sends the server nothing",
    async (visibility) => {
      const { pairs, inputs, destroyed } = browserApis();
      const status = { ...french, visibility };
      const requests = serve(status);
      const user = userEvent.setup();
      const post = await openedPost(status);

      await user.click(await within(post).findByRole("button", { name: "Translate" }));

      expect(await within(post).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
      expect(pairs).toEqual([["fr", "en"]]);
      expect(inputs).toEqual(["Bonjour tout le monde"]);
      expect(requests).toEqual([]);
      expect(destroyed).toHaveBeenCalledOnce();
    },
  );

  test("leaves a post to the provider that may receive it first, as the operator chose", async () => {
    const { pairs } = browserApis();
    const requests = serve(french, { providers: [provider], respond: () => HttpResponse.json(providerAnswer) });
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("Hello everyone")).toBeInTheDocument();
    expect(requests).toHaveLength(1);
    expect(pairs).toEqual([]);
  });

  test("takes over a followers-only post that the provider may not receive", async () => {
    const { pairs } = browserApis();
    const status: Status = { ...french, visibility: "private" };
    const requests = serve(status, { providers: [provider] });
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
    expect(pairs).toEqual([["fr", "en"]]);
    expect(requests).toEqual([]);
  });
});

describe("after the server refuses", () => {
  test.each<[string, () => Response]>([
    [
      "a post that no provider may receive, such as a public post of an author limited to followers",
      () => problem(422, "/api/v1/problems/translation-unavailable", "Unavailable"),
    ],
    [
      "a language that no provider translates",
      () => problem(422, "/api/v1/problems/translation-unsupported-language", "Unsupported"),
    ],
    ["the monthly limit", () => problem(429, "/api/v1/problems/translation-limit", "Too many")],
    ["a provider without a usable answer", () => problem(503, "/api/v1/problems/translation-failed", "Failed")],
    ["a failure of the server", () => problem(500, "about:blank", "It broke")],
  ])("the browser translates the post, for %s", async (_, respond) => {
    const { pairs } = browserApis();
    brands("Google Chrome");
    const requests = serve(french, { providers: [provider], respond });
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
    expect(within(post).getByText("Machine translated by your browser (Google Chrome)")).toBeInTheDocument();
    expect(within(post).queryByRole("alert")).not.toBeInTheDocument();
    expect(requests).toHaveLength(1);
    expect(pairs).toEqual([["fr", "en"]]);
  });

  test("the browser does not take over a post that the server does not show the reader", async () => {
    const { pairs } = browserApis();
    serve(french, { providers: [provider], respond: () => problem(404, "about:blank", "Not found") });
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent(/couldn't be translated/);
    expect(pairs).toEqual([]);
  });

  test("a browser without the API says the server's refusal", async () => {
    serve(french, {
      providers: [provider],
      respond: () => problem(422, "/api/v1/problems/translation-unavailable", "Unavailable"),
    });
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent("This server can't translate this post.");
  });

  test("the next press goes to the browser directly, with the press that the model's download needs", async () => {
    let creates = 0;
    browserApis({
      availability: "downloadable",
      create: () => {
        creates += 1;
        // The wait for the server used up the first press.
        return creates === 1
          ? Promise.reject(new DOMException("Requires a user gesture", "NotAllowedError"))
          : Promise.resolve();
      },
    });
    const requests = serve(french, {
      providers: [provider],
      respond: () => problem(429, "/api/v1/problems/translation-limit", "Too many"),
    });
    const user = userEvent.setup();
    const post = await openedPost();
    const button = await within(post).findByRole("button", { name: "Translate" });

    await user.click(button);
    expect(await within(post).findByRole("alert")).toHaveTextContent(/didn't allow this translation/);
    await user.click(button);

    expect(await within(post).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
    expect(requests).toHaveLength(1);
  });
});

describe("the translation", () => {
  test("keeps mentions, hashtags, links, shortcodes, and code as they are, and never sends them", async () => {
    const { inputs } = browserApis({
      answer: (input) => input.replace("Salut", "Hi").replace("regarde", "look at").replace("et", "and"),
    });
    const status: Status = {
      ...french,
      content:
        '<p>Salut <a class="mention" href="https://example.com/@carol">@carol</a>, regarde <a class="hashtag" href="https://example.com/tags/my">#my</a> et <code>x := 1</code> :blob:</p><pre><code>garde le code</code></pre>',
    };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    const mention = await within(post).findByRole("link", { name: /@carol/ });
    expect(mention).toHaveAttribute("href", "https://example.com/@carol");
    expect(mention).toHaveClass("mention");
    expect(within(post).getByRole("link", { name: /#my/ })).toHaveAttribute("href", "https://example.com/tags/my");
    expect(mention.parentElement?.textContent).toMatch(/^Hi @carol.*, look at #my.* and x := 1 :blob:$/);
    expect(within(post).getByText("garde le code", { selector: "pre code" })).toBeInTheDocument();
    expect(inputs).toEqual(["Salut ⟦1⟧, regarde ⟦2⟧ et ⟦3⟧ ⟦4⟧"]);
  });

  test("keeps a decoration around its words, wherever the translation puts them", async () => {
    browserApis({ answer: () => "Read ⟦1⟧this⟦/1⟧ now" });
    const status: Status = { ...french, content: "<p>Lis <strong>ceci</strong> maintenant</p>" };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    const strong = await within(post).findByText("this");
    expect(strong.tagName).toBe("STRONG");
    expect(strong.parentElement?.textContent).toBe("Read this now");
  });

  // As Chrome's en to ar, which turned the markers into plain numbers, and its de to ja, which added a `⟧`.
  test.each<[string, string]>([
    ["loses its markers", "Hi 1 2 look 2 3 now"],
    ["repeats a marker", "⟦1⟧ ⟦1⟧ ⟦2⟧⟦/2⟧ ⟦3⟧"],
    ["breaks a marker", "Hi ⟦1⟧⟧ look ⟦2⟧this⟦/2⟧ ⟦3⟧"],
  ])("translates between the markers when the output %s", async (_, broken) => {
    const { inputs } = browserApis({ answer: (input) => (input.includes("⟦") ? broken : `EN ${input}`) });
    const status: Status = {
      ...french,
      content:
        '<p>Salut <a href="https://example.com/@carol">@carol</a>, lis <strong>ceci</strong> maintenant <a href="https://example.com/x">x</a></p>',
    };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    const strong = await within(post).findByText("EN ceci");
    expect(strong.tagName).toBe("STRONG");
    expect(inputs.slice(1)).toEqual(["Salut", ", lis", "ceci", "maintenant"]);
    expect(strong.parentElement?.textContent).toMatch(/^EN Salut @carol.*EN , lis EN ceci EN maintenant x/);
  });

  test("shows markup in the output as text, and never as elements", async () => {
    browserApis({ answer: () => '<img src=x onerror="alert(1)"><script>alert(2)</script><b>bold</b>' });
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    const body = await within(post).findByText(/<b>bold<\/b>/);
    expect(within(body).queryByRole("img")).not.toBeInTheDocument();
    expect(within(body).queryByText("bold", { selector: "b" })).not.toBeInTheDocument();
    expect(body.textContent).toBe('<img src=x onerror="alert(1)"><script>alert(2)</script><b>bold</b>');
  });

  test("names the browser that translated, and says so to a screen reader", async () => {
    browserApis();
    brands("Not:A-Brand", "Chromium", "Google Chrome");
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("Machine translated by your browser (Google Chrome)")).toBeInTheDocument();
    expect(within(post).getByRole("button", { name: "Show original" })).toBeInTheDocument();
  });

  test("says only that it was the browser when the browser gives no name that is known", async () => {
    browserApis();
    brands("Not:A-Brand", "Chromium");
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("Machine translated by your browser")).toBeInTheDocument();
  });

  test("translates the content warning and the poll, and goes back to the original", async () => {
    const { inputs } = browserApis();
    const status: Status = {
      ...french,
      spoiler_text: "Attention :blob:",
      poll: { ...pollFixture, options: [{ title: "Oui" }, { title: "Non" }] },
    };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    await within(post).findByRole("button", { name: "Show original" });
    expect(inputs).toEqual(["Bonjour tout le monde", "Attention ⟦1⟧", "Oui", "Non"]);
    expect(within(post).getByText("EN Attention :blob:")).toBeInTheDocument();

    await user.click(within(post).getByRole("button", { name: "Show original" }));
    expect(within(post).queryByText(/^EN /)).not.toBeInTheDocument();
  });

  test("stays in memory: nothing of it reaches the browser's storage", async () => {
    browserApis();
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));
    await within(post).findByText("EN Bonjour tout le monde");

    for (const storage of [localStorage, sessionStorage]) {
      for (let index = 0; index < storage.length; index += 1) {
        expect(storage.getItem(storage.key(index) ?? "")).not.toContain("EN Bonjour");
      }
    }
  });
});

describe("the model's download", () => {
  test("shows its progress on the button while it is under way", async () => {
    let finish: () => void = () => undefined;
    const waiting = new Promise<void>((resolve) => {
      finish = resolve;
    });
    let report: ((event: { loaded: number }) => void) | undefined;
    browserApis({ availability: "downloadable" });
    vi.stubGlobal("Translator", {
      availability: () => Promise.resolve("downloadable"),
      create: async (options: { monitor: (monitor: Monitor) => void }) => {
        options.monitor({
          addEventListener: (_, listener) => {
            report = listener;
          },
        });
        await waiting;
        return { translate: () => Promise.resolve("Hello everyone"), destroy: () => undefined };
      },
    });
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    const button = await within(post).findByRole("button", { name: "Translate" });
    await user.click(button);
    await waitFor(() => {
      expect(report).toBeDefined();
    });
    report?.({ loaded: 0.4 });

    expect(await within(post).findByText("Downloading the translation model… 40%")).toBeInTheDocument();
    report?.({ loaded: 1 });
    expect(await within(post).findByText("Translating…")).toBeInTheDocument();

    finish();
    expect(await within(post).findByText("Hello everyone")).toBeInTheDocument();
  });

  test("shows no progress for a model that is already there", async () => {
    const create = vi.fn((options: { monitor?: unknown }) => {
      expect(options.monitor).toBeUndefined();
      return Promise.resolve({ translate: () => Promise.resolve("Hello"), destroy: () => undefined });
    });
    vi.stubGlobal("Translator", { availability: () => Promise.resolve("available"), create });
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("Hello")).toBeInTheDocument();
    expect(create).toHaveBeenCalledOnce();
  });
});

describe("a failure", () => {
  test.each<[string, () => Promise<void>, RegExp]>([
    [
      "a download that fails",
      () => Promise.reject(new DOMException("Download failed", "NetworkError")),
      /couldn't get its translation model ready/,
    ],
    [
      "a model that does not start",
      () => Promise.reject(new DOMException("Model not available", "NotSupportedError")),
      /couldn't get its translation model ready/,
    ],
    [
      "a browser that refuses, or a press that lapsed during another download",
      () => Promise.reject(new DOMException("Requires a user gesture", "NotAllowedError")),
      /didn't allow this translation/,
    ],
  ])("says so for %s, keeps the original, and can try again", async (_, create, message) => {
    browserApis({ availability: "downloadable", create });
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent(message);
    expect(within(post).getByText("Bonjour tout le monde")).toBeInTheDocument();
    expect(within(post).getByRole("button", { name: "Translate" })).toBeEnabled();
  });

  test("does not show an output whose markers are made up, even when translated piece by piece", async () => {
    browserApis({ answer: () => "Hi ⟦9⟧" });
    const status: Status = { ...french, content: '<p>Salut <a href="https://example.com/@carol">@carol</a></p>' };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent(/couldn't translate this post/);
    expect(within(post).getByRole("link", { name: /@carol/ }).parentElement?.textContent).toMatch(/^Salut @carol/);
  });

  test("says so when the translator fails on the way", async () => {
    vi.stubGlobal("Translator", {
      availability: () => Promise.resolve("available"),
      create: () =>
        Promise.resolve({
          translate: () => Promise.reject(new DOMException("Filtered", "NotReadableError")),
          destroy: () => undefined,
        }),
    });
    serve(french);
    const user = userEvent.setup();
    const post = await openedPost();

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent(/couldn't translate this post/);
  });
});

describe("a post of no known language", () => {
  const unknown: Status = { ...french, language: "und" };

  test("is translated from the language that the detector finds", async () => {
    const { pairs } = browserApis();
    detectorApi([{ detectedLanguage: "fr", confidence: 0.99 }], "available");
    serve(unknown);
    const user = userEvent.setup();
    const post = await openedPost(unknown);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText("EN Bonjour tout le monde")).toBeInTheDocument();
    expect(pairs).toEqual([["fr", "en"]]);
  });

  test.each<[string, { detectedLanguage: string; confidence: number }[], Availability, RegExp]>([
    ["finds nothing", [{ detectedLanguage: "und", confidence: 0.9 }], "available", /couldn't tell which language/],
    ["is unsure", [{ detectedLanguage: "fr", confidence: 0.2 }], "available", /couldn't tell which language/],
    [
      "finds the reader's language",
      [{ detectedLanguage: "en", confidence: 0.9 }],
      "available",
      /already in your language/,
    ],
    [
      "finds a language with no translation",
      [{ detectedLanguage: "fr", confidence: 0.9 }],
      "unavailable",
      /can't translate this post's language/,
    ],
  ])("gives no translation when the detector %s", async (_, results, availability, message) => {
    browserApis({ availability: "available" });
    detectorApi(results, "available");
    // The pair is only asked once the language is known; before that the action depends on the detector.
    vi.stubGlobal("Translator", { availability: () => Promise.resolve(availability), create: vi.fn() });
    serve(unknown);
    const user = userEvent.setup();
    const post = await openedPost(unknown);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByRole("alert")).toHaveTextContent(message);
  });
});

describe("a post with no words", () => {
  test("is the author's own text and asks the browser nothing", async () => {
    const { pairs } = browserApis();
    const status: Status = { ...french, content: '<p><a href="https://example.com/@carol">@carol</a> 123 !</p>' };
    serve(status);
    const user = userEvent.setup();
    const post = await openedPost(status);

    await user.click(await within(post).findByRole("button", { name: "Translate" }));

    expect(await within(post).findByText(/author's own text/)).toBeInTheDocument();
    expect(pairs).toEqual([]);
  });
});

describe("markersIntact", () => {
  test.each<[string, string, boolean]>([
    ["⟦1⟧ a ⟦2⟧b⟦/2⟧", "⟦2⟧B⟦/2⟧ A ⟦1⟧", true],
    ["no markers", "none either", true],
    ["⟦1⟧ a", "A", false],
    ["⟦1⟧ a", "⟦1⟧ ⟦1⟧ A", false],
    ["a", "⟦1⟧ A", false],
    ["⟦1⟧a⟦/1⟧", "⟦/1⟧A⟦1⟧", false],
    ["⟦1⟧⟦2⟧a⟦/2⟧⟦/1⟧", "⟦1⟧⟦2⟧A⟦/1⟧⟦/2⟧", false],
    ["⟦1⟧a⟦/1⟧", "⟦1⟧A", false],
  ])("%s to %s is %s", (fragment, translation, intact) => {
    expect(markersIntact(fragment, translation)).toBe(intact);
  });
});

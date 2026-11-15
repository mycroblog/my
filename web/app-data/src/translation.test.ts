// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { fakeAppData, json, type Handler } from "./test/fake";
import {
  serverTranslates,
  translateStatus,
  TranslationRefusal,
  translationRefusalOf,
  type InstanceTranslation,
  type StatusTranslation,
  type TranslationRefusalReason,
} from "./translation";

const translated = (language: string): StatusTranslation => ({
  language,
  source_language: "ja",
  content: `<p>[${language}] 小さなサーバー</p>`,
  spoiler_text: "",
  media_attachments: [],
  poll_options: [],
  provider: { id: "fake", name: "Demo Translate" },
});

/** A server that translates every post, and what it received: `METHOD path` and the JSON body of each request. */
function translating(answer: (target: string) => Response | Promise<Response> = (t) => json(200, translated(t))) {
  const sent: { request: string; body: unknown }[] = [];
  const handler: Handler = async (request) => {
    const body = (await request.json()) as { target_language: string };
    sent.push({ request: `${request.method} ${new URL(request.url).pathname}`, body });
    return answer(body.target_language);
  };
  return { sent, ...fakeAppData(handler) };
}

const problem = (status: number, type?: string) =>
  json(status, { ...(type === undefined ? {} : { type: `/api/v1/problems/${type}` }), title: "Problem", status });

/** Why translating post 1 was refused, when the server answers `answer`. */
async function refusalOf(answer: () => Response | Promise<Response>): Promise<TranslationRefusalReason> {
  const { data } = translating(answer);
  const error: unknown = await translateStatus(data, "1", "en").then(
    () => undefined,
    (e: unknown) => e,
  );
  if (!(error instanceof TranslationRefusal)) throw new Error(`not a TranslationRefusal: ${String(error)}`);
  expect(translationRefusalOf(error.cause)).toBe(error.reason);
  return error.reason;
}

test("a translation reaches the server as a POST of the target language, and resolves to its answer", async () => {
  const { data, sent } = translating();
  expect(await translateStatus(data, "1", "en")).toEqual(translated("en"));
  expect(sent).toEqual([{ request: "POST /api/v1/statuses/1/translation", body: { target_language: "en" } }]);
});

test("asking again for the same post, version and language asks the server once", async () => {
  const { data, sent } = translating();
  await translateStatus(data, "1", "en", "2026-11-14T03:00:00Z");
  expect(await translateStatus(data, "1", "en", "2026-11-14T03:00:00Z")).toEqual(translated("en"));
  expect(sent).toHaveLength(1);
});

test("two requests at once for the same translation ask the server once", async () => {
  const { data, sent } = translating();
  const [a, b] = await Promise.all([translateStatus(data, "1", "en"), translateStatus(data, "1", "en")]);
  expect(a).toEqual(b);
  expect(sent).toHaveLength(1);
});

test("another language, another version (an edit) or another post asks the server again", async () => {
  const { data, sent } = translating();
  await translateStatus(data, "1", "en", "v1");
  await translateStatus(data, "1", "fr", "v1");
  await translateStatus(data, "1", "en", "v2");
  await translateStatus(data, "2", "en", "v1");
  expect(sent.map((s) => [s.request, s.body])).toEqual([
    ["POST /api/v1/statuses/1/translation", { target_language: "en" }],
    ["POST /api/v1/statuses/1/translation", { target_language: "fr" }],
    ["POST /api/v1/statuses/1/translation", { target_language: "en" }],
    ["POST /api/v1/statuses/2/translation", { target_language: "en" }],
  ]);
});

test("a refusal is not kept: asking again asks the server again, and may succeed", async () => {
  let refuse = true;
  const { data, sent } = translating((t) => (refuse ? problem(503, "translation-failed") : json(200, translated(t))));
  const refused: unknown = await translateStatus(data, "1", "en").catch((e: unknown) => e);
  expect(refused).toBeInstanceOf(TranslationRefusal);
  refuse = false;
  expect(await translateStatus(data, "1", "en")).toEqual(translated("en"));
  expect(sent).toHaveLength(2);
});

test("each refusal of the server says why: the limit, the visibility, the language, or the providers failing", async () => {
  expect(await refusalOf(() => problem(429, "translation-limit"))).toBe("limit");
  expect(await refusalOf(() => problem(422, "translation-unavailable"))).toBe("visibility");
  expect(await refusalOf(() => problem(422, "translation-unsupported-language"))).toBe("language");
  expect(await refusalOf(() => problem(503, "translation-failed"))).toBe("failed");
});

test("a server that fails otherwise, or does not answer, is a failure the browser may take over", async () => {
  expect(await refusalOf(() => problem(500))).toBe("failed");
  expect(await refusalOf(() => problem(502))).toBe("failed");
  expect(await refusalOf(() => Promise.reject(new TypeError("Failed to fetch")))).toBe("failed");
});

test("a post the reader may not read, or a request refused for its authorization, is denied", async () => {
  expect(await refusalOf(() => problem(404))).toBe("denied");
  expect(await refusalOf(() => problem(401))).toBe("denied");
  expect(await refusalOf(() => problem(403))).toBe("denied");
});

test("the server translates a visibility only when a provider of the instance may receive it", () => {
  const offer: InstanceTranslation = {
    browser: false,
    providers: [{ id: "fake", name: "Demo Translate", visibilities: ["public", "local", "unlisted"] }],
  };
  expect(serverTranslates(offer, "public")).toBe(true);
  expect(serverTranslates(offer, "unlisted")).toBe(true);
  expect(serverTranslates(offer, "private")).toBe(false);
  expect(serverTranslates(offer, "direct")).toBe(false);
  const second = { id: "own", name: "Own", visibilities: ["private" as const] };
  expect(serverTranslates({ ...offer, providers: [...offer.providers, second] }, "private")).toBe(true);
  expect(serverTranslates({ browser: true, providers: [] }, "public")).toBe(false);
  expect(serverTranslates(undefined, "public")).toBe(false);
});

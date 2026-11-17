// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import type { FakeOptions } from "@my/e2e/fake";
import { fakeData } from "./test/data";
import {
  serverTranslates,
  translateStatus,
  TranslationRefusal,
  translationRefusalOf,
  type InstanceTranslation,
  type TranslationRefusalReason,
} from "./translation";

type Provider = NonNullable<NonNullable<FakeOptions["translation"]>["provider"]>;

/**
 * A server whose provider translates every post it may receive, or refuses as `provider` says, and what it received;
 * `welcome` and `ridge` are public posts of the scenario, in Japanese, and `followers` one for its author's followers.
 */
function translating(provider: Provider = "answers") {
  const { fake, data, requests } = fakeData({ translation: { provider } });
  const sent = fake.script("POST /statuses/{id}/translation");
  const [welcome, ridge, followers] = ["hana-welcome", "yuki-ridge", "ren-followers"].map(fake.idOf) as [
    string,
    string,
    string,
  ];
  return { fake, data, requests, bodies: () => sent.seen.map((s) => s.body), welcome, ridge, followers };
}

/** Why translating a post was refused. */
async function refusalOf(translation: Promise<unknown>): Promise<TranslationRefusalReason> {
  const error: unknown = await translation.then(
    () => undefined,
    (e: unknown) => e,
  );
  if (!(error instanceof TranslationRefusal)) throw new Error(`not a TranslationRefusal: ${String(error)}`);
  expect(translationRefusalOf(error.cause)).toBe(error.reason);
  return error.reason;
}

/** Why translating a post was refused by a server whose provider refuses as `provider` says. */
const refusedBy = (provider: Provider) => {
  const { data, welcome } = translating(provider);
  return refusalOf(translateStatus(data, welcome, "en"));
};

/** Why translating a post was refused when the request fails as `fail` says. */
const failedWith = (fail: number | "network") => {
  const { fake, data, welcome } = translating();
  fake.script("POST /statuses/{id}/translation", { fail });
  return refusalOf(translateStatus(data, welcome, "en"));
};

test("a translation reaches the server as a POST of the target language, and resolves to its answer", async () => {
  const { data, requests, bodies, welcome } = translating();
  const translated = await translateStatus(data, welcome, "en");
  expect(translated).toMatchObject({
    language: "en",
    source_language: "ja",
    provider: { id: "fake", name: "Demo Translate" },
  });
  expect(translated.content).toContain("[en] mycroblog のデモサーバーへようこそ");
  expect(requests).toEqual([`POST /statuses/${welcome}/translation`]);
  expect(bodies()).toEqual([{ target_language: "en" }]);
});

test("asking again for the same post, version and language asks the server once", async () => {
  const { data, requests, welcome } = translating();
  const first = await translateStatus(data, welcome, "en", "2026-11-14T03:00:00Z");
  expect(await translateStatus(data, welcome, "en", "2026-11-14T03:00:00Z")).toEqual(first);
  expect(requests).toHaveLength(1);
});

test("two requests at once for the same translation ask the server once", async () => {
  const { data, requests, welcome } = translating();
  const [a, b] = await Promise.all([translateStatus(data, welcome, "en"), translateStatus(data, welcome, "en")]);
  expect(a).toEqual(b);
  expect(requests).toHaveLength(1);
});

test("another language, another version (an edit) or another post asks the server again", async () => {
  const { data, requests, bodies, welcome, ridge } = translating();
  await translateStatus(data, welcome, "en", "v1");
  await translateStatus(data, welcome, "fr", "v1");
  await translateStatus(data, welcome, "en", "v2");
  await translateStatus(data, ridge, "en", "v1");
  expect(requests).toEqual([
    `POST /statuses/${welcome}/translation`,
    `POST /statuses/${welcome}/translation`,
    `POST /statuses/${welcome}/translation`,
    `POST /statuses/${ridge}/translation`,
  ]);
  expect(bodies().map((b) => b.target_language)).toEqual(["en", "fr", "en", "en"]);
});

test("a refusal is not kept: asking again asks the server again, and may succeed", async () => {
  const { fake, data, requests, welcome } = translating();
  fake.script("POST /statuses/{id}/translation", {
    times: 1,
    fail: { type: "/api/v1/problems/translation-failed", title: "The translation provider did not answer", status: 503 },
  });
  const refused: unknown = await translateStatus(data, welcome, "en").catch((e: unknown) => e);
  expect(refused).toBeInstanceOf(TranslationRefusal);
  expect((await translateStatus(data, welcome, "en")).language).toBe("en");
  expect(requests).toHaveLength(2);
});

test("each refusal of the server says why: the limit, the visibility, the language, or the providers failing", async () => {
  expect(await refusedBy("limit")).toBe("limit");
  expect(await refusedBy("unavailable")).toBe("visibility");
  // No provider receives a post for its author's followers.
  const { data, followers } = translating();
  expect(await refusalOf(translateStatus(data, followers, "en"))).toBe("visibility");
  expect(await refusedBy("language")).toBe("language");
  expect(await refusedBy("failed")).toBe("failed");
});

test("a server that fails otherwise, or does not answer, is a failure the browser may take over", async () => {
  expect(await failedWith(500)).toBe("failed");
  expect(await failedWith(502)).toBe("failed");
  expect(await failedWith("network")).toBe("failed");
});

test("a post the reader may not read, or a request refused for its authorization, is denied", async () => {
  // A post the server does not have is one the reader may not read: a 404.
  const { data } = translating();
  expect(await refusalOf(translateStatus(data, "00000000-0000-7000-8000-00000000ffff", "en"))).toBe("denied");
  expect(await failedWith(401)).toBe("denied");
  expect(await failedWith(403)).toBe("denied");
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

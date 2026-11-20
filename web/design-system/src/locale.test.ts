// SPDX-License-Identifier: AGPL-3.0-only

// The reader of a package's catalog (#3094): a page holds the en messages with the catalog and loads another locale's
// only when a reader asks for it, so a locale not loaded yet reads en, never the ids of its messages; one load of a
// locale is one request however many parts ask for it at once, and a load that failed leaves the locale as it was.

import { expect, test } from "bun:test";
import { catalogReader, type Catalog, type Loaders } from "./locale";

type Id = "greeting" | "farewell";

const catalog: Catalog = { source: { greeting: "Hello", farewell: "Goodbye" }, locales: ["en", "ja", "fr"] };
const ja = { greeting: "こんにちは", farewell: "さようなら" };

/** Loaders whose `ja` answers when `answer` is called, and that count the requests each locale got. */
function heldLoaders() {
  const asked: Record<string, number> = {};
  // The answer of the next request of `ja`, which a request made before `answer` waits for.
  let next = Promise.withResolvers<typeof ja>();
  const loaders: Loaders = {
    ja: () => {
      asked["ja"] = (asked["ja"] ?? 0) + 1;
      return next.promise;
    },
    fr: () => {
      asked["fr"] = (asked["fr"] ?? 0) + 1;
      return Promise.resolve({ greeting: "Bonjour", farewell: "Au revoir" });
    },
  };
  const answer = (fail = false) => {
    const now = next;
    next = Promise.withResolvers<typeof ja>();
    if (fail) now.reject(new Error("test: the chunk did not load"));
    else now.resolve(ja);
  };
  return { loaders, asked, answer };
}

test("a locale not loaded yet reads en, never the ids of its messages", () => {
  const reader = catalogReader<Id>(catalog);

  expect(reader.has("ja")).toBe(false);
  expect(reader("ja").t("greeting")).toBe("Hello");
  expect(reader("ja-JP").t("farewell")).toBe("Goodbye");
  expect(reader("en").t("greeting")).toBe("Hello");
  // A locale the catalog does not have reads en as well.
  expect(reader("de").t("greeting")).toBe("Hello");
});

test("a locale loaded reads in its messages, and so does a region of it", async () => {
  const reader = catalogReader<Id>(catalog);
  const { loaders, answer } = heldLoaders();

  const loading = reader.load("ja", loaders);
  // Until the messages arrive, the locale reads en.
  expect(reader.has("ja")).toBe(false);
  expect(reader("ja").t("greeting")).toBe("Hello");
  answer();
  await loading;

  expect(reader.has("ja")).toBe(true);
  expect(reader.has("ja-JP")).toBe(true);
  expect(reader("ja").t("greeting")).toBe("こんにちは");
  expect(reader("ja-JP").t("farewell")).toBe("さようなら");
  // The others are not loaded with it.
  expect(reader.has("fr")).toBe(false);
  expect(reader("fr").t("greeting")).toBe("Hello");
});

test("loads of one locale at once make one request, and a locale loaded already makes none", async () => {
  const reader = catalogReader<Id>(catalog);
  const { loaders, asked, answer } = heldLoaders();

  const loads = [reader.load("ja", loaders), reader.load("ja", loaders), reader.load("ja-JP", loaders)];
  answer();
  await Promise.all(loads);
  await reader.load("ja", loaders);

  expect(asked).toEqual({ ja: 1 });
  expect(reader("ja").t("greeting")).toBe("こんにちは");
});

test("a locale the page added reads in its messages without a request", async () => {
  const reader = catalogReader<Id>(catalog);
  const { loaders, asked } = heldLoaders();

  reader.add({ ja });
  await reader.load("ja", loaders);

  expect(reader.has("ja")).toBe(true);
  expect(reader("ja").t("greeting")).toBe("こんにちは");
  expect(asked).toEqual({});
});

test("a load that failed leaves the locale not loaded, in en, and a later load asks again", async () => {
  const reader = catalogReader<Id>(catalog);
  const held = heldLoaders();

  const failed = reader.load("ja", held.loaders).then(
    () => "loaded",
    () => "failed",
  );
  held.answer(true);
  await failed;

  expect(reader.has("ja")).toBe(false);
  expect(reader("ja").t("greeting")).toBe("Hello");

  const again = reader.load("ja", held.loaders);
  held.answer();
  await again;

  expect(held.asked).toEqual({ ja: 2 });
  expect(reader.has("ja")).toBe(true);
  expect(reader("ja").t("greeting")).toBe("こんにちは");
});

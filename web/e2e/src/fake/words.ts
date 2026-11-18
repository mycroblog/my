// SPDX-License-Identifier: AGPL-3.0-only

// The server's words of the problems the fake answers, in the language a request asks for: the server localizes a
// problem's title and detail from Accept-Language, and the handlers write theirs in English. A request in Japanese
// gets the server's own Japanese sentence (locales/ja/server/errors.json) for each English one listed here, word for
// word; a sentence that is not listed stays as its handler wrote it.

import type { Schema } from "./spec";

/** The server's sentences in Japanese, by their English: each a message of locales/<lang>/server/errors.json. */
const japanese: Readonly<Record<string, string>> = {
  // errors.unprocessable
  "This could not be done.": "この操作はできませんでした。",
  // errors.invalid_value
  "This value is not valid. Check what you entered.": "正しくない値です。入力した内容を確かめてください。",
  // errors.media_corrupt
  "The file could not be read as an image. Choose another file.":
    "ファイルを画像として読み込めませんでした。ほかのファイルを選んでください。",
};

/** Whether `request` asks for Japanese, as the fake's settings read it for the labels they word. */
const asksJapanese = (request: Request) =>
  request.headers.get("Accept-Language")?.toLowerCase().startsWith("ja") === true;

/** `problem` as the server words it for `request`: its title and its detail in Japanese where the request asks so. */
export function worded(problem: Schema<"Problem">, request: Request): Schema<"Problem"> {
  if (!asksJapanese(request)) return problem;
  const { title, detail } = problem;
  return {
    ...problem,
    title: japanese[title] ?? title,
    ...(detail === undefined ? {} : { detail: japanese[detail] ?? detail }),
  };
}

// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from "bun:test";
import { copyFindings, copyText, loadWords, parseWords } from "./copy";
import { extractArgs, parseMessage } from "./icu";

const words = loadWords();

/** The tier and rule of each finding of an en message, or of a ja message against its en message. */
function lint(locale: string, message: string, en?: string): string[] {
  const ast = parseMessage(message);
  let source: { text: string; missing: string[] } | undefined;
  if (en !== undefined) {
    const enAst = parseMessage(en);
    const got = extractArgs(ast);
    source = { text: copyText(enAst), missing: [...extractArgs(enAst).keys()].filter((n) => !got.has(n)) };
  }
  return copyFindings(words, locale, copyText(ast), source).map((f) => `${f.tier} ${f.rule}`);
}

test("the word decisions of the copy role are rules: a word to avoid blocks, the word to use does not", () => {
  expect(lint("en", "Log in to continue")).toEqual(["block words"]);
  expect(lint("en", "Stops it logging in.")).toEqual(["block words"]);
  expect(lint("en", "Sign in to continue")).toEqual([]);
  expect(lint("en", "Login providers")).toEqual([]);
  expect(lint("ja", "サインインしてください")).toEqual(["block words"]);
  expect(lint("ja", "ログインしてください")).toEqual([]);
  expect(lint("ja", "もう一度お試しください")).toEqual(["block words"]);
  expect(lint("ja", "アカウントを作る")).toEqual(["block words"]);
});

test("a word line out of shape, or a pattern that does not compile, is refused with the line", () => {
  expect(() => parseWords('- en: write "x", avoid `(` — why — from — 2026-11-12 — held 0 / missed 0')).toThrow(
    /decisions\.md:1:/,
  );
  expect(() => parseWords('- en: use "x" instead of `y` — why')).toThrow(/a word decision is/);
  expect(
    parseWords("# copy\n\n- Prose about ja: is not a word line — why — from — 2026-11-12 — held 0 / missed 0"),
  ).toEqual([]);
});

test("emoji block; ©, ® and ™ are not emoji", () => {
  expect(lint("en", "Posted 🎉")).toEqual(["block emoji"]);
  expect(lint("en", "© {year} Example™")).toEqual([]);
});

test("dashes, and a colon after a label, are waivable; a colon that leads into a list or a value is not one", () => {
  expect(lint("en", "{page} – {instance}")).toEqual(["waive dash"]);
  expect(lint("en", "{room} — open now")).toEqual(["waive dash"]);
  expect(lint("en", "Closed - for now")).toEqual(["waive dash"]);
  expect(lint("en", "Up to {max, number} letters (a to z) and well-known words")).toEqual([]);
  expect(lint("ja", "閉鎖しました ― {server}")).toEqual(["waive dash"]);
  expect(lint("en", "The moderators' answer:")).toEqual(["waive colon"]);
  expect(lint("ja", "モデレーターからの回答:")).toEqual(["waive colon"]);
  expect(lint("en", "{app} wants to use your account with these permissions:")).toEqual([]);
  expect(lint("en", "Error: {message}")).toEqual([]);
});

test("ja: an auxiliary verb in kanji is waivable, a formal noun after kana is to answer", () => {
  expect(lint("ja", "確認して下さい。")).toEqual(["waive kana"]);
  expect(lint("ja", "ご協力頂きありがとうございます。")).toEqual(["waive kana"]);
  expect(lint("ja", "もう一度試して見てください。")).toEqual(["waive kana"]);
  expect(lint("ja", "保存して置きます。")).toEqual(["waive kana"]);
  expect(lint("ja", "確認してください。画面で見る。山頂に行く。")).toEqual([]);
  expect(lint("ja", "保存した時に通知します。")).toEqual(["answer formal-noun"]);
  expect(lint("ja", "保存したときに通知します。登録時と時間と事前の記事。")).toEqual([]);
});

test("ja against en: URLs and code spans block, a dropped argument is waivable, numbers are to answer", () => {
  const en = "Read https://example.com/docs, set `MY_HOST` and wait {minutes} minutes. Up to 1,000 posts.";
  expect(
    lint("ja", "https://example.com/docs を読み、`MY_HOST` を設定して {minutes} 分待ちます。投稿は 1000 件まで。", en),
  ).toEqual([]);
  expect(
    lint("ja", "https://example.org/docs を読み、`MY_HOST` を設定して {minutes} 分待ちます。投稿は 1000 件まで。", en),
  ).toEqual(["block url"]);
  expect(
    lint("ja", "https://example.com/docs を読み、MY_HOST を設定して {minutes} 分待ちます。投稿は 1000 件まで。", en),
  ).toEqual(["block code"]);
  expect(
    lint("ja", "https://example.com/docs を読み、`MY_HOST` を設定して少し待ちます。投稿は 1000 件まで。", en),
  ).toEqual(["waive placeholder"]);
  expect(
    lint("ja", "https://example.com/docs を読み、`MY_HOST` を設定して {minutes} 分待ちます。投稿は 100 件まで。", en),
  ).toEqual(["answer number"]);
  expect(lint("ja", "1 つの通貨で 10 万円まで。", "One currency, up to 100,000 yen.")).toEqual([]);
  expect(lint("ja", "1 回だけ", "once")).toEqual([]);
});

test("the copy lint reads en and ja only", () => {
  expect(copyFindings(words, "de", "Log in — jetzt 🎉")).toEqual([]);
});

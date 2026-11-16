// SPDX-License-Identifier: AGPL-3.0-only
import { expect, test } from "bun:test";
import {
  ADMIN,
  branchTexts,
  copyFindings,
  copyText,
  errorOf,
  hasNextStep,
  loadNoStep,
  loadWords,
  nextStepErrors,
  parseNoStep,
  parseWords,
} from "./copy";
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

test("a variant of a recorded term is refused with the term to use; the right sense of a near word passes", () => {
  const found = (locale: string, text: string) => copyFindings(words, locale, text).map((f) => f.message);
  expect(found("ja", "このサーバーはサービスを終了しました。")).toEqual([expect.stringContaining('use "閉鎖"')]);
  expect(found("ja", "サーバーをクローズします。")).toEqual([expect.stringContaining('use "閉鎖"')]);
  expect(found("ja", "運営者からのお知らせ")).toEqual([expect.stringContaining('use "管理者"')]);
  expect(found("ja", "閉鎖を告知しました。")).toEqual([expect.stringContaining('use "予告"')]);
  expect(found("ja", "アカウントを再有効化する")).toEqual([expect.stringContaining('use "アカウントをもとに戻す"')]);
  expect(found("ja", "サーバーから追放する")).toEqual([expect.stringContaining('use "BAN"')]);
  expect(found("ja", "サポーター制度を始める")).toEqual([expect.stringContaining('use "サポーターのプラン"')]);
  expect(found("ja", "課金機能を使う")).toEqual([expect.stringContaining('use "サポーターのプラン"')]);
  expect(found("ja", "GitHub とひも付ける")).toEqual([expect.stringContaining('use "連携"')]);
  expect(found("ja", "紐づけを外す")).toEqual([expect.stringContaining('use "連携"')]);
  expect(found("ja", "記事の題を入力")).toEqual([expect.stringContaining('use "タイトル"')]);
  expect(found("ja", "カバーを追加")).toEqual([expect.stringContaining('use "カバー画像"')]);
  expect(
    found(
      "ja",
      "メンテナンスは {time} ごろに終了する予定です。イベントは終了しました。問題が発生しました。話題。元に戻す。カバー画像を追加。テストモードです (課金されません)。",
    ),
  ).toEqual([]);
});

test("system words block in the strings users read and pass in those only administrators read", () => {
  expect(lint("en", "Call the API to change the role.")).toEqual(["block words", "block words"]);
  expect(lint("en", "The file is 2 bytes too long.")).toEqual(["block words"]);
  expect(lint("en", "Start a conversation")).toEqual(["block words"]);
  expect(lint("ja", "ロールを保存できません。")).toEqual(["block words"]);
  expect(lint("ja", "スキーマが違います。会話を開く。")).toEqual(["block words", "block words"]);
  expect(lint("en", "Approve the {role} of {conversation}")).toEqual([]);
  const admin = (locale: string, text: string) => copyFindings(words, locale, text, undefined, true);
  expect(admin("en", "The role cannot be saved.")).toEqual([]);
  expect(admin("ja", "ロールを保存できません。")).toEqual([]);
  // A decided word holds for administrators too: only a "to users" line spares them.
  expect(admin("ja", "運営者のロール").map((f) => f.rule)).toEqual(["words"]);
  expect(ADMIN.test("API error title for HTTP 422 when a role breaks a rule. Read by administrators.")).toBe(true);
  expect(ADMIN.test("Read by other servers' operators.")).toBe(true);
  expect(ADMIN.test("Error shown when an administrator disabled the app.")).toBe(false);
});

test("a word line holds several terms after `; write`, and `, to users` marks it to spare admin strings", () => {
  const parsed = parseWords(
    '- ja, to users: write "a", avoid `x`, `y`; write "b", avoid `z` — why — from — 2026-11-16 — held 0 / missed 0',
  );
  expect(parsed.map((w) => [w.locale, w.use, w.avoid.map(String), w.users])).toEqual([
    ["ja", '"a"', ["/x/mu", "/y/mu"], true],
    ["ja", '"b"', ["/z/mu"], true],
  ]);
  expect(() =>
    parseWords('- ja: write "a", avoid `x`; write "b" — why — from — 2026-11-16 — held 0 / missed 0'),
  ).toThrow(/decisions\.md:1: a word decision is/);
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

test("an error is a key with an error segment, or a client key that failed; its detail is part of it", () => {
  expect(errorOf("server", "errors.role.invalid_detail")).toEqual({
    group: "server:errors.role.invalid",
    retry: false,
  });
  expect(errorOf("server", "errors.status.rejected.detail")?.group).toBe("server:errors.status.rejected");
  expect(errorOf("server", "payments.error.declined")).toEqual({
    group: "server:payments.error.declined",
    retry: false,
  });
  expect(errorOf("appfeatures", "compose.load_failed")).toEqual({
    group: "appfeatures:compose.load_failed",
    retry: true,
  });
  expect(errorOf("appcomponents", "column.failed_more")?.retry).toBe(true);
  expect(errorOf("appfeatures", "auth.error.title")).toBeUndefined();
  expect(errorOf("appfeatures", "notifications.kind.supporter_payment_failed")).toBeUndefined();
  expect(errorOf("server", "push.supporter_payment_failed")).toBeUndefined();
  expect(errorOf("appfeatures", "compose.schedule.failed.restricted")).toBeUndefined();
});

test("a next step: ja asks with ください or says the limit; en starts with an imperative or says what is needed", () => {
  for (const text of [
    "もう一度試してください。",
    "先にブロックを解除してください。",
    "説明は # 文字までです。",
    "この操作にはログインが必要です。",
    "まだログインして、データをエクスポートできます。",
    "いまは何もしなくてかまいません。",
  ])
    expect([text, hasNextStep("ja", text)]).toEqual([text, true]);
  for (const text of [
    "このメールアドレスは確認済みです。",
    "ホストか共同ホストだけがこの操作をできます。",
    "投稿を読み込めませんでした。",
  ])
    expect([text, hasNextStep("ja", text)]).toEqual([text, false]);
  for (const text of [
    "Try again.",
    "You block this account. Unblock it first.",
    "If it is being prepared, wait until it is ready.",
    "The name must be 1 to 50 characters.",
    "A description can have at most # characters.",
    "You can still sign in and export your data.",
    "{reason, select}\nReject the request instead.",
  ])
    expect([text, hasNextStep("en", text)]).toEqual([text, true]);
  for (const text of [
    "The email address is already verified.",
    "You cannot create invites.",
    "The posts could not be loaded.",
  ])
    expect([text, hasNextStep("en", text)]).toEqual([text, false]);
  // G18: a failure of the client, whose next step is the retry button beside it.
  expect(hasNextStep("ja", "投稿を読み込めませんでした。", true)).toBe(true);
  expect(hasNextStep("en", "The posts could not be loaded.", true)).toBe(true);
  expect(hasNextStep("en", "Deleting the post did not finish.", true)).toBe(true);
  expect(hasNextStep("ja", "このサーバーの翻訳サービスが応答していません。", true)).toBe(false);
});

/** One message of an error, as check.ts gives it to nextStepErrors. */
const msg = (locale: string, key: string, icu: string) => ({ locale, key, branches: branchTexts(parseMessage(icu)) });

test("an error with no next step is refused unless the reviewed list holds it; a stale entry is refused", () => {
  const error = (en: string, ja: string, retry = false, checked = true) => ({
    texts: [msg("en", "errors.x", en), msg("ja", "errors.x", ja)],
    retry,
    checked,
  });
  const errors = new Map([
    ["server:errors.a", error("The email address is already verified.", "このメールアドレスは確認済みです。")],
    ["server:errors.b", error("Unblock it first.", "このアカウントをブロックしています。")],
    ["server:errors.c", error("Try again.", "もう一度試してください。")],
    ["server:errors.d", error("You cannot create invites.", "招待を作成できません。", false, false)],
  ]);
  expect(nextStepErrors(errors, new Map())).toEqual([
    expect.stringMatching(/^server:errors\.a: the error says no next step in en and ja; .*no-step\.json/),
    expect.stringMatching(/^server:errors\.b: the error says no next step in ja;/),
  ]);
  const reason = "It reports a state that already holds: nothing is left for the reader to do.";
  expect(
    nextStepErrors(
      errors,
      new Map([
        ["server:errors.a", reason],
        ["server:errors.b", reason],
        ["server:errors.c", reason],
        ["server:errors.gone_key", reason],
      ]),
    ),
  ).toEqual([
    "tools/i18n/no-step.json: server:errors.c says its next step now; remove it from the list",
    "tools/i18n/no-step.json: server:errors.gone_key is not an error of the catalogs (errorOf); remove it",
  ]);
});

test("the branches of a message are its select and plural choices, with the text around them", () => {
  expect(branchTexts(parseMessage("Try again."))).toEqual(new Map([["", "Try again."]]));
  expect(
    branchTexts(parseMessage("Saved. {reason, select, url {Check the URL.} other {It breaks a rule.}} {n} left")),
  ).toEqual(
    new Map([
      ["url", "Saved. Check the URL. {n} left"],
      ["other", "Saved. It breaks a rule. {n} left"],
    ]),
  );
  expect([
    ...branchTexts(
      parseMessage("{reason, select, max {{n, plural, one {At most # item.} other {At most # items.}}} other {No.}}"),
    ).keys(),
  ]).toEqual(["max/one", "max/other", "other"]);
});

test("each branch of an error needs a step, its own or the same branch's of a sibling, or a list entry", () => {
  const detail = (locale: string, other: string) =>
    msg(
      locale,
      "errors.y_detail",
      `{reason, select, url {${locale === "en" ? "Check the URL." : "URL を確かめてください。"}} other {${other}}}`,
    );
  const title = (locale: string, text: string) => msg(locale, "errors.y", text);
  const y = (titles: [string, string], others: [string, string]) =>
    new Map([
      [
        "server:errors.y",
        {
          texts: [title("en", titles[0]), title("ja", titles[1]), detail("en", others[0]), detail("ja", others[1])],
          retry: false,
          checked: true,
        },
      ],
    ]);
  const stateful = y(["The relay cannot be saved.", "保存できません。"], ["It breaks a rule.", "規則に反しています。"]);
  expect(nextStepErrors(stateful, new Map())).toEqual([
    expect.stringMatching(/^server:errors\.y_detail#other: this branch says no next step in en and ja;/),
  ]);
  // The title's step holds for every branch of its detail.
  expect(
    nextStepErrors(
      y(["Try again.", "もう一度試してください。"], ["It breaks a rule.", "規則に反しています。"]),
      new Map(),
    ),
  ).toEqual([]);
  // A step in one locale only leaves the other locale lacking.
  expect(
    nextStepErrors(
      y(["The relay cannot be saved.", "保存できません。"], ["Change the value.", "規則に反しています。"]),
      new Map(),
    ),
  ).toEqual([expect.stringMatching(/^server:errors\.y_detail#other: this branch says no next step in ja;/)]);
  const reason = "The fallback of a rule the server checks: no step fits every case it covers.";
  expect(nextStepErrors(stateful, new Map([["server:errors.y_detail#other", reason]]))).toEqual([]);
  expect(nextStepErrors(stateful, new Map([["server:errors.y", reason]]))).toEqual([]);
  expect(
    nextStepErrors(
      stateful,
      new Map([
        ["server:errors.y_detail#other", reason],
        ["server:errors.y_detail#url", reason],
      ]),
    ),
  ).toEqual([
    "tools/i18n/no-step.json: server:errors.y_detail#url is no branch of an error that lacks a next step (it has one now, or is gone); remove it",
  ]);
});

test("the reviewed list gives each error once, under a reason that says why no step is possible", () => {
  expect(parseNoStep('{"It reports a state that already holds, nothing to do.": ["server:errors.a"]}')).toEqual(
    new Map([["server:errors.a", "It reports a state that already holds, nothing to do."]]),
  );
  expect(() => parseNoStep('{"No step.": ["server:errors.a"]}')).toThrow(/at least 40 characters/);
  expect(() =>
    parseNoStep(
      '{"It reports a state that already holds, nothing to do.": ["server:errors.a"], "A rule of the server refuses it, which the reader cannot change.": ["server:errors.a"]}',
    ),
  ).toThrow(/listed twice/);
  expect(() => parseNoStep('["server:errors.a"]')).toThrow(/an object of/);
  expect(loadNoStep().size).toBeGreaterThan(0);
});

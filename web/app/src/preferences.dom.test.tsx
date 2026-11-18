// SPDX-License-Identifier: AGPL-3.0-only

// The app following the account's preferences (#3199): the language it reads in and says the page is in (ADR-0009 §2,
// §5), the time zone its times read in, and what the device keeps of both, so that the app opened again paints first
// as the account chose, before its preferences are read, and forgets once the reader signed out. An account that
// follows the browser is told the browser's language and the device's zone where the server holds others, once and
// without a word to the reader. A reader without an account follows the browser, and nothing is asked of the server
// about them or sent to it. Yuki reads; the browser's language is the test's (en unless it says otherwise),
// and the device's zone is Tokyo's, which the tests set and put back. What each part does with a zone is its own test
// (the design system's zone and time, app-components' drafts, post-details and edit-history); what a preference does
// to a timeline and to a post's link card is the features'.

import { afterAll, beforeAll, describe, expect, test } from "bun:test";
import { localInput, scheduleTime } from "@my/app-components";
import { preferencesQuery, updatePreferences, viewerQuery } from "@my/app-data";
import { dateFormat } from "@my/design-system";
import { scenario, type FakeOptions } from "@my/e2e/fake";
import { readDeviceValue, writeDeviceValue } from "@my/platform";
import { cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { main, renderApp, type AppOptions } from "./test/app";
import { newPost, paste } from "./test/composer";
import { ownDevice } from "./test/device";
import { feedOf, postOf } from "./test/posts";
import { fakeApp, type FakeApp } from "./test/server";

const device = process.env.TZ;

beforeAll(() => {
  process.env.TZ = "Asia/Tokyo";
});

afterAll(() => {
  if (device === undefined) delete process.env.TZ;
  else process.env.TZ = device;
});

const newYork = "America/New_York";
const tokyo = "Asia/Tokyo";

/** The app at `at`, alone in the document: opened again on the same device when a test opened one before. */
function opened(at: string, options: AppOptions = {}) {
  cleanup();
  return renderApp(at, options);
}

/** The language and the direction the page says it is in. */
const page = () => ({ lang: document.documentElement.lang, dir: document.documentElement.dir });

/** The title of the screen in place, as its heading says it. */
const heading = () =>
  main()
    .queryAllByRole("heading", { level: 1 })
    .map((title) => title.textContent)
    .join();

/** Waits until the page says it is in `lang` and the screen's title reads `title`. */
const reads = (lang: string, title: string) =>
  waitFor(() => {
    if (page().lang !== lang || heading() !== title)
      throw new Error(`test: the page is in "${page().lang}" under "${heading()}", not in "${lang}" under "${title}"`);
  });

/** Whether the app has the preferences the server answered. */
const hasPreferences = ({ data }: Pick<FakeApp, "data">) =>
  data.queryClient.getQueryData(preferencesQuery(data).queryKey) !== undefined;

/** The title of the lists' screen, a screen that shows little, in each language. */
const lists = { en: "Lists", ja: "リスト" };

const japanese = { locale: "ja", locale_follows_browser: false } as const;

describe("the language", () => {
  test("is the one the account chose, over the browser's: in the app's words, in the document's title and on the page", async () => {
    ownDevice();
    const app = await opened("/lists", { preferences: japanese });

    await reads("ja", lists.ja);
    expect(page().dir).toBe("ltr");
    expect(app.title()).toContain(lists.ja);
  });

  test("changes with the account's choice while the app is open: a language no catalog has follows the browser, a region reads from its language's catalog", async () => {
    ownDevice();
    const app = await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);

    await updatePreferences(app.data, { locale: "fr" });
    await reads("en", lists.en);

    await updatePreferences(app.data, { locale: "ja-JP" });
    await reads("ja-JP", lists.ja);

    await updatePreferences(app.data, { locale_follows_browser: true });
    await reads("en", lists.en);
    expect(app.title()).toContain(lists.en);
    // The same app, on the same screen: nothing was loaded again.
    expect(app.path()).toBe("/lists");
  });

  test("in a browser of another language, the account's choice still wins, and without one the browser's does", async () => {
    ownDevice();
    const app = await opened("/lists", { locale: "ja", preferences: { locale: "en", locale_follows_browser: false } });
    await reads("en", lists.en);

    await updatePreferences(app.data, { locale: "fr" });
    await reads("ja", lists.ja);
  });

  test("a language chosen shows at once, before the server answers, and one the server refuses is taken back", async () => {
    ownDevice();
    // The server holds the browser's language and the device's zone: the app has nothing of its own to send it.
    const app = await opened("/lists", { preferences: { locale: "en", time_zone: tokyo } });
    await waitFor(() => {
      if (!hasPreferences(app)) throw new Error("test: the preferences are not read yet");
    });
    await reads("en", lists.en);
    const sent = app.fake.script("PATCH /preferences", { hold: "request", fail: 500 });

    const refused = updatePreferences(app.data, { locale: "ja" }).catch((error: unknown) => error);
    // The fake holds the request: the server has not answered.
    await reads("ja", lists.ja);
    expect((await sent.arrived()).body).toEqual({ locale: "ja" });
    sent.release();

    expect(await refused).toBeInstanceOf(Error);
    await reads("en", lists.en);
  });
});

/** Sam's post, edited once, and Yuki's reply to it: a thread whose every part says a time. */
const thread = {
  people: scenario.people,
  posts: [
    {
      key: "bread",
      author: "sam",
      text: "Bread at ten on Saturday.",
      earlier: ["Bread at nine on Saturday."],
      visibility: "public",
      language: "en",
    },
    {
      key: "butter",
      author: "yuki",
      replyTo: "bread",
      text: "I will bring butter.",
      visibility: "public",
      language: "en",
    },
  ],
} as const satisfies FakeOptions["seed"];

const inNewYork = { time_zone: newYork, time_zone_follows_browser: false } as const;

/** What a clock of `timeZone` reads at `at`, as `locale` writes it: "10:40 PM", "22:40". */
const clock = (at: string | undefined, timeZone: string, locale = "en") =>
  dateFormat(locale, timeZone, { hour: "numeric", minute: "2-digit" })
    .format(new Date(at ?? Number.NaN))
    .replace(/\s/g, " ");

/** What `time` says to a reader: its words, then what it says when asked for the full date. */
const said = (time: HTMLElement) => `${time.textContent} / ${time.title}`.replace(/\s/g, " ");

/** The thread of Sam's post on `on`, drawn: the moments of its two posts, and the times its posts show. */
async function threadOn(on: FakeApp) {
  const app = await opened(on.fake.postPath("bread"), { on });
  const moments = await Promise.all(
    ["bread", "butter"].map(async (key) => {
      const { data } = await on.data.api.GET("/statuses/{id}", { params: { path: { id: on.fake.idOf(key) } } });
      return data?.created_at;
    }),
  );
  // The thread is the screen's one feed, whatever the language names it in; a change of language draws it anew.
  const feed = () => main().getByRole("feed");
  await main().findByRole("feed");
  /** The one time the post of `author` that holds `text` shows. */
  const timeOf = (author: string, text: string) => within(postOf(feed(), author, text)).getByRole("time");
  return {
    app,
    feed,
    posted: { bread: moments[0], butter: moments[1] },
    /** The line under the post the thread is about, which says when it was posted. */
    details: () => said(timeOf("Sam Rivera", "Bread at ten")),
    /** The stamp of the reply. */
    stamp: () => said(timeOf("Yuki 雪", "I will bring butter.")),
  };
}

/**
 * Whether `all` says `words` as words of their own, no digit before them and none after: "1:01 PM" is not said by
 * "11:01 PM", which is what New York's clock reads when Tokyo's reads it.
 */
const tells = (all: string, words: string) =>
  new RegExp(String.raw`(?<!\d)${words.replace(/[.*+?^${}()|[\]\\]/g, String.raw`\$&`)}(?!\d)`).test(all);

/** Waits until `read()` says `words`, as words of their own. */
const says = (read: () => string, words: string) =>
  waitFor(() => {
    if (!tells(read(), words)) throw new Error(`test: "${read()}" does not say "${words}"`);
  });

describe("the time zone", () => {
  test("is the account's: the clock of a post's stamp, of the post's details and of its edit history", async () => {
    ownDevice();
    const { app, feed, posted, details, stamp } = await threadOn(fakeApp({ seed: thread, preferences: inNewYork }));

    await says(details, clock(posted.bread, newYork));
    await says(stamp, clock(posted.butter, newYork));
    // Tokyo's clock, the device's, reads another hour.
    expect(tells(details(), clock(posted.bread, tokyo))).toBe(false);

    await app.user.click(
      within(postOf(feed(), "Sam Rivera", "Bread at ten")).getByRole("button", { name: "Edited, show history" }),
    );
    const history = await screen.findByRole("dialog", { name: "Edit history" });
    const { data: edits } = await app.data.api.GET("/statuses/{id}/history", {
      params: { path: { id: app.fake.idOf("bread") } },
    });
    // The versions newest first, each at its own moment.
    const versions = (edits?.items ?? []).toReversed();
    expect(versions).toHaveLength(2);
    await waitFor(() => {
      const times = within(history).queryAllByRole("time").map(said);
      if (
        times.length !== 2 ||
        !versions.every((version, i) => tells(times[i] ?? "", clock(version.created_at, newYork)))
      )
        throw new Error(`test: the history's times are ${times.join(", ")}`);
    });
  });

  test("is the device's while the account follows the browser, and changes with the account's choice with no reload", async () => {
    ownDevice();
    const on = fakeApp({ seed: thread, preferences: inNewYork });
    const { app, posted, details, stamp } = await threadOn(on);
    await says(details, clock(posted.bread, newYork));

    // The account keeps its zone for what has no browser; the app reads the device's.
    expect(await updatePreferences(app.data, { time_zone_follows_browser: true })).toMatchObject({
      time_zone: newYork,
    });
    await says(details, clock(posted.bread, tokyo));
    await says(stamp, clock(posted.butter, tokyo));

    await updatePreferences(app.data, { time_zone: "Asia/Kolkata" });
    await says(details, clock(posted.bread, "Asia/Kolkata"));
    expect(app.path()).toBe(on.fake.postPath("bread"));
  });

  test("is the clock the composer schedules a post on, and the server gets the moment the time names there", async () => {
    ownDevice();
    const app = await opened("/lists", { preferences: inNewYork });
    await waitFor(() => {
      if (!hasPreferences(app)) throw new Error("test: the preferences are not read yet");
    });
    const sent = app.fake.script("PUT /drafts/{id}/schedule");
    const composer = await newPost(app);
    await paste(app, composer.text, "Bread at ten on Saturday.");

    await app.user.click(within(composer.dialog).getByRole("button", { name: "Schedule", pressed: false }));
    const field = await screen.findByLabelText<HTMLInputElement>("Date and time");
    const described = (field.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => document.getElementById(id)?.textContent ?? "");
    expect(described.join(" / ")).toMatch(/America\/New_York · GMT-[45]/);
    // A month from now at 15:30 UTC, the late morning in New York, written as its clock reads then.
    const chosen = new Date(Date.now() + 30 * 86_400_000);
    chosen.setUTCHours(15, 30, 0, 0);
    fireEvent.change(field, { target: { value: localInput(chosen, newYork) } });
    await app.user.click(screen.getByRole("button", { name: "Set time" }));

    const scheduled = await screen.findByRole("dialog", { name: "New post" });
    await says(() => scheduled.textContent, `Scheduled for ${scheduleTime(chosen, "en", newYork)} (America/New_York`);
    await app.user.click(within(scheduled).getByRole("button", { name: "Schedule" }));

    expect((await sent.arrived()).body).toEqual({ publish_at: chosen.toISOString() });
  });
});

describe("the app opened again on the device", () => {
  /** The app at `at` on a server whose answer about the preferences is held: `read.release()` lets it go. */
  async function openedUnread(at: string, options: FakeOptions = {}) {
    const on = fakeApp(options);
    const read = on.fake.script("GET /preferences", { hold: "request" });
    const app = await opened(at, { on });
    await read.arrived();
    if (hasPreferences(on)) throw new Error("test: the preferences are read, though their answer is held");
    return { on, app, read };
  }

  test("paints first in the language and the zone the account chose, before its preferences are read", async () => {
    ownDevice();
    const chose = { seed: thread, preferences: { ...japanese, ...inNewYork } };
    // The first run reads the preferences, and the device keeps what the account chose.
    const first = await threadOn(fakeApp(chose));
    await reads("ja", "投稿");
    await says(first.details, clock(first.posted.bread, newYork, "ja"));

    const on = fakeApp(chose);
    const read = on.fake.script("GET /preferences", { hold: "request" });
    const again = await threadOn(on);
    await read.arrived();

    // Drawn, and the server has not said what the account prefers.
    expect(hasPreferences(on)).toBe(false);
    expect(page()).toEqual({ lang: "ja", dir: "ltr" });
    expect(heading()).toBe("投稿");
    expect(tells(again.details(), clock(again.posted.bread, newYork, "ja"))).toBe(true);
  });

  test("turns from what the device kept to what the account prefers now, once its preferences are read", async () => {
    ownDevice();
    await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);

    // The account follows the browser by now: another device changed it.
    const { read } = await openedUnread("/lists");
    expect([page().lang, heading()]).toEqual(["ja", lists.ja]);
    read.release();

    await reads("en", lists.en);
  });

  test("paints first in the browser's language once the account follows the browser again", async () => {
    ownDevice();
    const first = await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);
    await updatePreferences(first.data, { locale_follows_browser: true });
    await reads("en", lists.en);

    // Whatever the server would answer, the first paint has nothing of the earlier choice.
    await openedUnread("/lists", { preferences: japanese });

    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });

  test("keeps the account's choice while signing out did not finish, and forgets it once the reader signed out", async () => {
    ownDevice();
    /** Presses the confirmation's own button, ログアウトする, on the settings' menu in Japanese. */
    const signOut = async (app: Awaited<ReturnType<typeof opened>>) => {
      await reads("ja", "設定");
      const asking = await screen.findByRole("alertdialog", { name: "このブラウザからログアウトしますか？" });
      await app.user.click(within(asking).getByRole("button", { name: "ログアウトする" }));
    };
    const refused = await opened("/settings?sign_out=true", { preferences: japanese, revocation: "fails" });
    await signOut(refused);
    await screen.findByRole("alert");

    // Still signed in: the app opened again paints as the account chose.
    const kept = await openedUnread("/settings?sign_out=true", { preferences: japanese });
    expect([page().lang, heading()]).toEqual(["ja", "設定"]);
    kept.read.release();
    await signOut(kept.app);
    await waitFor(() => {
      if (kept.app.left().length === 0) throw new Error("test: the app did not leave yet");
    });

    // Signed out: whoever opens the app next on this device, its first paint is the browser's.
    await openedUnread("/lists", { preferences: japanese });
    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });

  test("is the account's alone: once the reader is read as another account it is dropped, before their preferences are read, and for good", async () => {
    ownDevice();
    await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);

    // Sam signs in on the device Yuki read on: whose account it is the app knows once it read the viewer.
    const sam = await openedUnread("/lists", { viewer: "sam" });
    await waitFor(() => {
      if (sam.on.data.queryClient.getQueryData(viewerQuery(sam.on.data).queryKey) === undefined)
        throw new Error("test: the viewer is not read yet");
    });
    await reads("en", lists.en);
    expect(hasPreferences(sam.on)).toBe(false);

    // Dropped, not only passed over: Yuki's next run has nothing kept to paint first with.
    await openedUnread("/lists", { preferences: japanese });
    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });

  test("is not used when it names no account, as a build before this one kept it", async () => {
    ownDevice();
    await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);
    // What the device keeps, without whose it is. This test alone reads the kept value: it has to make an older one.
    const kept = JSON.parse(readDeviceValue("my:reader-choice") ?? "null") as Record<string, unknown> | null;
    expect(Object.keys(kept ?? {})).toContain("account");
    writeDeviceValue("my:reader-choice", JSON.stringify({ ...kept, account: undefined }));

    await openedUnread("/lists", { preferences: japanese });

    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });

  test("is kept when the app could not ask whether the browser has a session, and paints the reader's next run", async () => {
    ownDevice();
    await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);

    // The server did not answer when the app started: it runs as for a visitor, who may be Yuki still.
    await opened("/local", { visitor: true, unasked: true });
    await feedOf("Local timeline");

    await openedUnread("/lists", { preferences: japanese });
    expect([page().lang, heading()]).toEqual(["ja", lists.ja]);
  });
});

describe("an account that follows the browser", () => {
  const paris = "Europe/Paris";
  // What the server holds of the account's language and zone, and what the app, in English on a device in Tokyo, sends.
  const held = [
    [
      "another language and another zone",
      { locale: "fr", time_zone: paris },
      [{ locale: "en", locale_follows_browser: true, time_zone: tokyo, time_zone_follows_browser: true }],
    ],
    ["another language", { locale: "fr", time_zone: tokyo }, [{ locale: "en", locale_follows_browser: true }]],
    ["another zone", { locale: "en", time_zone: paris }, [{ time_zone: tokyo, time_zone_follows_browser: true }]],
    ["the browser's language and the device's zone", { locale: "en", time_zone: tokyo }, []],
  ] as const;

  for (const [what, preferences, sends] of held)
    test(`and whose server holds ${what} is sent ${sends.length === 0 ? "nothing" : "them once, still following"}`, async () => {
      ownDevice();
      const on = fakeApp({ preferences });
      const sent = on.fake.script("PATCH /preferences");
      const app = await opened("/lists", { on });
      if (sends.length > 0) await sent.served();
      else
        await waitFor(() => {
          if (!hasPreferences(on)) throw new Error("test: the preferences are not read yet");
        });

      // The app moves on with the preferences read: whatever it sends of them by itself, it has sent.
      await app.navigate("/bookmarks");
      await app.navigate("/lists");

      expect(sent.seen.map((request) => request.body)).toEqual([...sends]);
      const { data: stored } = await on.data.api.GET("/preferences");
      expect(stored).toMatchObject({
        locale: "en",
        locale_follows_browser: true,
        time_zone: tokyo,
        time_zone_follows_browser: true,
      });
    });

  test("no more: an account that chose its language and its zone is sent nothing, whatever the browser's are", async () => {
    ownDevice();
    const on = fakeApp({
      preferences: { locale: "ja", locale_follows_browser: false, time_zone: paris, time_zone_follows_browser: false },
    });
    const sent = on.fake.script("PATCH /preferences");
    const app = await opened("/lists", { on });
    await reads("ja", lists.ja);

    await app.navigate("/bookmarks");
    await app.navigate("/lists");

    expect(sent.seen).toHaveLength(0);
  });

  test("and whose update the server fails is not sent it again, though the account is read again", async () => {
    ownDevice();
    const on = fakeApp({ preferences: { locale: "fr", time_zone: paris } });
    const sent = on.fake.script("PATCH /preferences", { fail: 500 });
    const read = on.fake.script("GET /preferences");
    const app = await opened("/lists", { on });
    await sent.arrived();

    await on.data.queryClient.invalidateQueries({ queryKey: preferencesQuery(on.data).queryKey });
    await read.served(2);
    await app.navigate("/bookmarks");
    await app.navigate("/lists");

    expect(sent.seen).toHaveLength(1);
    // The reader saw nothing of it: the app reads in the browser's language all along.
    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });

  // The update is made from what the server last said, never from a change of the reader's that is still on its
  // way: Yuki chose Japanese, the browser reads English, and Yuki now chooses to follow the browser.
  describe("by a choice the reader just made", () => {
    /** The app with that choice made and held at the server: the app reads in English already. */
    async function following(outcome: "kept" | "refused") {
      ownDevice();
      const on = fakeApp({ preferences: { ...japanese, time_zone: tokyo } });
      const sent = on.fake.script("PATCH /preferences");
      const app = await opened("/lists", { on });
      await reads("ja", lists.ja);
      const held = on.fake.script("PATCH /preferences", {
        times: 1,
        hold: "request",
        ...(outcome === "refused" ? { fail: 500 } : {}),
      });
      const choice = updatePreferences(on.data, { locale_follows_browser: true }).catch((error: unknown) => error);
      await held.arrived();
      await reads("en", lists.en);
      return { on, app, sent, held, choice, bodies: () => sent.seen.map((request) => request.body) };
    }

    test("is sent the browser's language once the server answered the choice, and once only", async () => {
      const { app, sent, held, choice, bodies } = await following("kept");

      held.release();
      expect(await choice).toMatchObject({ locale: "ja", locale_follows_browser: true });
      await sent.served(2);
      await app.navigate("/bookmarks");
      await app.navigate("/lists");

      expect(bodies()).toEqual([{ locale_follows_browser: true }, { locale: "en", locale_follows_browser: true }]);
      expect([page().lang, heading()]).toEqual(["en", lists.en]);
    });

    test("is sent nothing when the server refused the choice: the account still holds its own language", async () => {
      const { on, app, held, choice, bodies } = await following("refused");

      held.release();
      expect(await choice).toBeInstanceOf(Error);
      await reads("ja", lists.ja);
      await app.navigate("/bookmarks");
      await app.navigate("/lists");

      expect(bodies()).toEqual([{ locale_follows_browser: true }]);
      const { data: stored } = await on.data.api.GET("/preferences");
      expect(stored).toMatchObject({ locale: "ja", locale_follows_browser: false });
    });
  });
});

describe("a reader without an account", () => {
  test("follows the browser, in its language and on the device's clock, and nothing is asked or sent of their preferences", async () => {
    ownDevice();
    // An account that chose Japanese and New York read on this device before.
    await opened("/lists", { preferences: { ...japanese, ...inNewYork } });
    await reads("ja", lists.ja);

    const on = fakeApp({ visitor: true, seed: thread });
    const asked = on.fake.script("GET /preferences");
    const sent = on.fake.script("PATCH /preferences");
    await opened("/local", { on });

    const feed = await feedOf("Local timeline");
    expect(page()).toEqual({ lang: "en", dir: "ltr" });
    expect(heading()).toBe("Local");
    const reply = within(postOf(feed, "Yuki 雪", "I will bring butter.")).getByRole("time");
    const moment = reply.getAttribute("datetime") ?? "";
    expect(tells(said(reply), clock(moment, tokyo))).toBe(true);
    expect(tells(said(reply), clock(moment, newYork))).toBe(false);
    await waitFor(() => {
      if (on.data.queryClient.isFetching() > 0) throw new Error("test: the server's answers are still on their way");
    });
    expect(asked.seen).toHaveLength(0);
    // The device is in Tokyo and the browser reads English, as no account does: the app tells the server of neither.
    expect(sent.seen).toHaveLength(0);
    expect(page().lang).toBe("en");
  });

  test("leaves nothing of an earlier account's choice on the device: the next first paint is the browser's", async () => {
    ownDevice();
    await opened("/lists", { preferences: japanese });
    await reads("ja", lists.ja);
    await opened("/local", { visitor: true });
    await feedOf("Local timeline");

    const on = fakeApp({ preferences: japanese });
    const read = on.fake.script("GET /preferences", { hold: "request" });
    await opened("/lists", { on });
    await read.arrived();

    expect(hasPreferences(on)).toBe(false);
    expect([page().lang, heading()]).toEqual(["en", lists.en]);
  });
});

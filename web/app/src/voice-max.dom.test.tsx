// SPDX-License-Identifier: AGPL-3.0-only

// How many people a voice chat room holds, on the fake API with the calls on fakes (ADR-0100 §3, §5), in en and ja:
// the host sets it when opening the room (a number sent, an empty field sending nothing, one out of range refused
// before sending, one the server refuses marked); a room's title says how many of how many are in it; a full room says
// so under its description before the reader joins, and a join it refuses next to Join; and in the call, while more are
// in the room than its maximum, the notice says so, in the host's words to the host of a room they lowered. What the
// settings send is the feature's (web/app-features/src/call/room-form.dom.test.tsx); how the count looks when full,
// the stories' (voice.stories.tsx: RoomFull, OverMax, HostingOverMax).

import { joinRoom } from "@my/app-features";
import { describe, expect, test } from "bun:test";
import { screen, waitFor } from "@testing-library/react";
import { main, renderApp, words, type TestApp } from "./test/app";
import { callsOnFakes, type FakeCalls } from "./test/call";
import { fakeApp } from "./test/server";

/** What the screens say of the maximum, in each locale the done-when names. */
const said = {
  en: {
    name: "Name",
    field: "How many can be in the room",
    hint: "2 to 10. Leave it empty for this server's maximum, 10.",
    rule: "Enter a number from 2 to 10 for how many can be in the room, or leave it empty.",
    open: "Open and join",
    join: "Join",
    speakers: "Speakers",
    count: (n: number, max: number) => `${String(n)} of ${String(max)} in the room`,
    full: "The room is full. Try again when fewer people are in it.",
    over: (max: number) =>
      `More people are in the room than its maximum, ${String(max)}. No one was removed, and no one new can join until fewer than ${String(max)} are in it.`,
    hostOver: (max: number) =>
      `More people are in the room than the maximum you set, ${String(max)}. No one was removed, and no one new can join until fewer than ${String(max)} are in it. To let new people join now, raise the maximum.`,
  },
  ja: {
    name: "名前",
    field: "参加できる人数",
    hint: "2〜10 人。空欄のときは、このサーバーの上限の 10 人です。",
    rule: "参加できる人数には 2〜10 の数を入れるか、空欄にしてください。",
    open: "開いて参加する",
    join: "参加する",
    speakers: "話者",
    count: (n: number, max: number) => `${String(max)} 人中 ${String(n)} 人が参加中`,
    full: "ルームが満員です。人数が減ってから、もう一度試してください。",
    over: (max: number) =>
      `ルームの人数が上限の ${String(max)} 人を超えています。退出させられた人はいませんが、${String(max)} 人より少なくなるまで新しい人は参加できません。`,
    hostOver: (max: number) =>
      `ルームの人数が、あなたが設定した上限の ${String(max)} 人を超えています。退出させられた人はいませんが、${String(max)} 人より少なくなるまで新しい人は参加できません。新しい人がすぐ参加できるようにするには、上限を上げてください。`,
  },
} as const;

type Locale = keyof typeof said;
const locales = Object.keys(said) as Locale[];

/** Whether `first` comes before `then` in the page's order. */
const before = (first: Node, then: Node) =>
  (first.compareDocumentPosition(then) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** The room's title, the main's heading that holds its count. */
const title = (locale: Locale, n: number, max: number) =>
  main().getByRole("heading", {
    level: 2,
    name: (name) => name.replace(/\s+/g, " ").includes(said[locale].count(n, max)),
  });

/** Waits until the main says `sentence`, and returns its paragraphs that say it whole. */
async function saying(sentence: string): Promise<HTMLElement[]> {
  return waitFor(() => {
    const found = main()
      .queryAllByText((_, element) => element?.tagName === "P" && words(element).startsWith(sentence))
      .filter((p) => p.tagName === "P");
    if (found.length === 0)
      throw new Error(`test: the main does not say ${sentence}: ${words(screen.getByRole("main"))}`);
    return found;
  });
}

/** What the main says now, as one line. */
const mainSays = () => words(screen.getByRole("main"));

/** Waits until the main no longer says `sentence`. */
const saysNoLonger = (sentence: string) =>
  waitFor(() => {
    if (words(screen.getByRole("main")).includes(sentence)) throw new Error(`test: the main still says ${sentence}`);
  });

describe.each(locales)("opening a room, in %s", (locale) => {
  const w = said[locale];

  /** The form at /voice/new with the room's name written, and the count of the openings sent. */
  async function opening() {
    callsOnFakes();
    const on = fakeApp();
    const posts = on.fake.script("POST /voice/rooms");
    const app = await renderApp("/voice/new", { on, locale });
    const name = await main().findByRole("textbox", { name: w.name });
    await app.user.click(name);
    await app.user.paste("Alpha");
    const field = main().getByRole("textbox", { name: w.field });
    const submit = main().getByRole("button", { name: w.open });
    return { app, posts, field, submit };
  }

  /** Waits for the room the form opened, and returns its ID. */
  async function opened(app: TestApp): Promise<string> {
    await waitFor(() => {
      expect(app.path()).toMatch(/^\/voice\/(?!new$)[^/]+$/);
    });
    return app.path().split("/").at(-1) ?? "";
  }

  test("the field is empty, described by the server's maximum", async () => {
    const { field } = await opening();
    expect(field).toHaveProperty("value", "");
    const described = (field.getAttribute("aria-describedby") ?? "")
      .split(" ")
      .map((id) => document.getElementById(id)?.textContent);
    expect(described).toContain(w.hint);
  });

  test("a number entered goes with the room, which then holds it", async () => {
    const { app, posts, field, submit } = await opening();
    await app.user.click(field);
    await app.user.paste("5");
    await app.user.click(submit);
    const id = await opened(app);
    expect(posts.seen[0]?.body.max_participants).toBe(5);
    expect(await waitFor(() => title(locale, 1, 5))).toBeDefined();
    const { data } = await app.data.api.GET("/voice/rooms/{id}", { params: { path: { id } } });
    expect(data?.max_participants).toBe(5);
  });

  test("an empty field sends nothing of it, and the room holds the server's maximum", async () => {
    const { app, posts, submit } = await opening();
    await app.user.click(submit);
    await opened(app);
    expect(posts.seen).toHaveLength(1);
    expect("max_participants" in (posts.seen[0]?.body ?? {})).toBe(false);
    expect(await waitFor(() => title(locale, 1, 10))).toBeDefined();
  });

  test.each(["1", "11"])(
    "%s is refused before sending, its error under the field, which takes the focus",
    async (value) => {
      const { app, posts, field, submit } = await opening();
      await app.user.click(field);
      await app.user.paste(value);
      await app.user.click(submit);
      await waitFor(() => {
        expect(document.getElementById(`${field.id}-error`)?.textContent).toBe(w.rule);
      });
      expect(field.getAttribute("aria-invalid")).toBe("true");
      expect(app.focused()).toBe(field);
      expect(posts.seen).toHaveLength(0);
      expect(app.path()).toBe("/voice/new");
    },
  );

  test("a maximum the server refuses (422 max_participants) marks the field with the rule", async () => {
    const { app, field, submit } = await opening();
    app.fake.script("POST /voice/rooms", {
      fail: { type: "about:blank", title: "This value is not valid", status: 422, field: "max_participants" },
    });
    await app.user.click(field);
    await app.user.paste("6");
    await app.user.click(submit);
    await waitFor(() => {
      expect(document.getElementById(`${field.id}-error`)?.textContent).toBe(w.rule);
    });
    expect(field.getAttribute("aria-invalid")).toBe("true");
    await waitFor(() => {
      expect(app.focused()).toBe(field);
    });
    expect(app.path()).toBe("/voice/new");
  });
});

describe.each(locales)("a room before joining, in %s", (locale) => {
  const w = said[locale];
  const description = "Bread, bikes and whatever the week brought. Drop in, say hi, or just listen.";

  test("its title says how many of how many are in it", async () => {
    const on = fakeApp();
    const id = on.fake.voice.roomId(0);
    await renderApp(`/voice/${id}`, { on, locale });
    const heading = await waitFor(() => title(locale, 8, 10));
    expect(words(heading)).toContain("8/10");
    expect(mainSays()).not.toContain(w.full);
  });

  test("a full room says so under its description, before who is in it, and Join stays offered", async () => {
    callsOnFakes();
    const on = fakeApp({ voiceMax: { rooms: [8] } });
    const id = on.fake.voice.roomId(0);
    await renderApp(`/voice/${id}`, { on, locale });
    const heading = await waitFor(() => title(locale, 8, 8));
    expect(words(heading)).toContain("8/8");
    const [notice] = await saying(w.full);
    if (notice === undefined) throw new Error("test: no notice");
    expect(before(main().getByText(description), notice)).toBe(true);
    expect(before(notice, main().getByRole("heading", { level: 2, name: w.speakers }))).toBe(true);
    const join = main().getByRole("button", { name: w.join });
    expect(join.hasAttribute("disabled")).toBe(false);
    expect(join.getAttribute("aria-disabled")).not.toBe("true");
  });

  test("a join the server refuses as full (409) is said next to Join, and the reader is in no room", async () => {
    const calls = callsOnFakes();
    const on = fakeApp();
    const id = on.fake.voice.roomId(0);
    // The room filled meanwhile: the page read it with a place left.
    on.fake.script("POST /voice/rooms/{id}/participation", {
      fail: { type: "/api/v1/problems/voice-room-full", title: "The room is full", status: 409 },
    });
    const app = await renderApp(`/voice/${id}`, { on, locale });
    const join = await main().findByRole("button", { name: w.join });
    expect(mainSays()).not.toContain(w.full);
    await app.user.click(join);
    await calls.clock.advance(0);
    const [refusal, ...more] = await saying(w.full);
    if (refusal === undefined) throw new Error("test: no refusal");
    expect(more).toHaveLength(0);
    // Next to Join: after who is in the room, and before the button.
    expect(before(main().getByRole("heading", { level: 2, name: w.speakers }), refusal)).toBe(true);
    expect(before(refusal, main().getByRole("button", { name: w.join }))).toBe(true);
    expect(app.fake.voice.joined()).toBeNull();
  });

  test("a full room's own refusal (the fake's 409) is said next to Join too", async () => {
    const calls = callsOnFakes();
    const on = fakeApp({ voiceMax: { rooms: [8] } });
    const id = on.fake.voice.roomId(0);
    const app = await renderApp(`/voice/${id}`, { on, locale });
    await app.user.click(await main().findByRole("button", { name: w.join }));
    await calls.clock.advance(0);
    // The notice under the description, and the refusal.
    const refusal = await waitFor(async () => {
      const paragraphs = await saying(w.full);
      if (paragraphs.length < 2) throw new Error("test: the refusal is not said yet");
      return paragraphs.at(-1);
    });
    if (refusal === undefined) throw new Error("test: no refusal");
    expect(before(refusal, main().getByRole("button", { name: w.join }))).toBe(true);
    expect(app.fake.voice.joined()).toBeNull();
  });
});

/** Joins the fake's room at `index` as a screen's Join does, then opens its page in `locale`. */
async function inRoom(calls: FakeCalls, index: number, locale: Locale, on = fakeApp()) {
  const app = await renderApp("/lists", { on, locale });
  const id = app.fake.voice.roomId(index);
  await joinRoom(app.data, id);
  await calls.clock.advance(0);
  await app.navigate(`/voice/${id}`);
  await waitFor(() => {
    expect(app.fake.voice.joined()?.room).toBe(id);
  });
  return { app, id };
}

describe.each(locales)("in a room over its maximum, in %s", (locale) => {
  const w = said[locale];

  test("a listener reads that no one was removed and no one new joins, while more are in it than its maximum", async () => {
    const calls = callsOnFakes();
    const { app } = await inRoom(calls, 0, locale);
    expect(mainSays()).not.toContain(w.over(7));
    // The host lowers the maximum below the nine in the room.
    app.fake.voice.hostSetsMax(7);
    await saying(w.over(7));
    expect(await waitFor(() => title(locale, 9, 7))).toBeDefined();
    expect(mainSays()).not.toContain(w.hostOver(7));

    // Two leave: seven of seven is not over the maximum.
    app.fake.voice.participantLeaves(app.fake.accountId("ren"));
    await saying(w.over(7));
    app.fake.voice.participantLeaves(app.fake.accountId("kai"));
    await saysNoLonger(w.over(7));
    expect(await waitFor(() => title(locale, 7, 7))).toBeDefined();
  });

  test("the host who lowered it below the server's reads how to let new people join now", async () => {
    const calls = callsOnFakes();
    const { app, id } = await inRoom(calls, 2, locale);
    await app.data.api.PATCH("/voice/rooms/{id}", { params: { path: { id } }, body: { max_participants: 4 } });
    const [notice] = await saying(w.hostOver(4));
    if (notice === undefined) throw new Error("test: no notice");
    expect(mainSays()).not.toContain(w.over(4));
    expect(await waitFor(() => title(locale, 6, 4))).toBeDefined();
  });

  test("the host of a room at the server's maximum reads the other words: there is none to raise", async () => {
    const calls = callsOnFakes();
    const on = fakeApp();
    on.fake.script("GET /instance", { change: (instance) => ({ ...instance, voice_max_participants: 4 }) });
    const { app } = await inRoom(calls, 2, locale, on);
    app.fake.voice.hostSetsMax(4);
    await saying(w.over(4));
    expect(mainSays()).not.toContain(w.hostOver(4));
  });
});

test("the host's notice leads to the room's settings", async () => {
  const calls = callsOnFakes();
  const { app, id } = await inRoom(calls, 2, "en");
  await app.data.api.PATCH("/voice/rooms/{id}", { params: { path: { id } }, body: { max_participants: 4 } });
  await saying(said.en.hostOver(4));
  const link = main().getByRole("link", { name: "Room settings" });
  expect(link.getAttribute("href")).toBe(`/voice/${id}/settings`);
});

// SPDX-License-Identifier: AGPL-3.0-only

// The drafts and the scheduled posts as a feature: what their list shows in each state, what the composer shows of a
// post that is scheduled, that failed, that was saved elsewhere or that is gone from the server, and the schedule's
// form with the server's limits.

import { expect, test } from "bun:test";
import { defaultComposeSettings, defaultDraftLimits, useComposeSettings, type ServerDraft } from "@my/app-data";
import { readerZone, scheduleTime } from "@my/app-components";
import type { BoundFunctions, queries } from "@testing-library/dom";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import { stringsIn } from "../locale";
import { fakeData, pasteInto, renderFeature } from "../test/feature";
import { ComposerView } from "./composer";
import { DraftsView, type DraftsState } from "./drafts";
import type { ComposeSpecimen } from "./layer";
import { emptyDraft, type Draft, type Target } from "./model";
import { ScheduleTimeForm } from "./tools";
import { useComposer } from "./use-composer";

const hour = 3_600_000;
const day = 24 * hour;

/** A moment `ms` from now, as the server writes it. */
const from = (ms: number) => new Date(Date.now() + ms).toISOString();

/** A draft of the reader's, saved `savedHours` ago. */
const draft = (id: string, text: string, savedHours: number, more: Partial<ServerDraft> = {}): ServerDraft => ({
  id,
  created_at: from(-(savedHours + 1) * hour),
  updated_at: from(-savedHours * hour),
  text,
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  media: [],
  ...more,
});

const failedAt = from(-day);
const lists = {
  drafts: [
    draft("d1", "Walked along the river this morning.", 3, {
      media: [{ id: "m1" }, { id: "m2" }] as unknown as ServerDraft["media"],
    }),
    draft("d2", "Booked two seats. See you at the pier!", 5, {
      in_reply_to_id: "gone",
      last_schedule: { publish_at: failedAt, outcome: "failed", reason: "reply_unavailable", occurred_at: failedAt },
    }),
    draft("d3", "Which day works for the next photo walk?", 26, {
      poll: { options: ["Sat", "Sun"], expires_in: 86400 },
    }),
    draft("d4", "", 72, { media: [{ id: "m3" }] as unknown as ServerDraft["media"] }),
  ],
  scheduled: [
    draft("s1", "The drawing session is tomorrow at 2 pm.", 1, { scheduled_at: from(day) }),
    draft("s2", "Weekly photo thread is open.", 30, { scheduled_at: from(4 * day) }),
  ],
};

const none = () => undefined;

const follows = (before: Element, after: Element) =>
  (before.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING) !== 0;

/** The drafts' list in `state`, alone in the document, in `locale`: its dialog, and what it asked of its screen. */
function list(state: DraftsState, locale = "en") {
  cleanup();
  const asked: string[] = [];
  const { data } = fakeData();
  const user = renderFeature(
    data,
    <DraftsView
      state={state}
      layer={false}
      phone="dialog"
      onBack={() => asked.push("back")}
      onClose={none}
      onDismiss={none}
      onRetry={() => asked.push("retry")}
      onOpen={(id) => asked.push(`open ${id}`)}
      onUnschedule={none}
      onDelete={none}
    />,
    locale,
  );
  const dialog = screen.getByRole("dialog");
  return { user, asked, dialog, part: within(dialog), said: () => dialog.textContent };
}

/** The rows of the list named `name`, each by the start of what it says after what it does. */
const rows = (part: BoundFunctions<typeof queries>, name: string) =>
  within(part.getByRole("list", { name })).getAllByRole("listitem");

test("listed: the scheduled posts first, under one line of the reader's zone, then the drafts", () => {
  const { dialog, part, said } = list({ kind: "ready", ...lists });

  expect(screen.getByRole("dialog", { name: "Drafts and scheduled posts" }) === dialog).toBe(true);
  expect(part.getByRole("button", { name: "Back to the post" })).not.toBeNull();
  const scheduled = part.getByRole("heading", { name: /^Scheduled\s*2$/ });
  const drafts = part.getByRole("heading", { name: /^Drafts\s*4$/ });
  expect(follows(scheduled, drafts)).toBe(true);
  // One line of the zone, over the scheduled posts alone.
  const zone = `Times in ${readerZone("en")}`;
  expect(said().split(zone)).toHaveLength(2);
  expect(scheduled.parentElement?.textContent).toContain(zone);
  // Each list is named, the rows in the order given: the scheduled soonest first, the drafts newest first.
  expect(
    rows(part, "Scheduled posts").map((row) => /The drawing session|Weekly photo thread/.exec(row.textContent)?.[0]),
  ).toEqual(["The drawing session", "Weekly photo thread"]);
  expect(
    rows(part, "Drafts").map((row) => /Walked along|Booked two|Which day|No text/.exec(row.textContent)?.[0]),
  ).toEqual(["Walked along", "Booked two", "Which day", "No text"]);
});

test("a scheduled post reads by the time it goes out; a draft by what it carries and when it was saved", () => {
  const { part } = list({ kind: "ready", ...lists });
  const [soonest] = rows(part, "Scheduled posts");
  const [first, ...others] = rows(part, "Drafts");
  const [walked, booked, which, files] = [first, ...others].map((row) => (row === undefined ? undefined : within(row)));
  if (
    soonest === undefined ||
    walked === undefined ||
    booked === undefined ||
    which === undefined ||
    files === undefined
  )
    throw new Error("test: the lists miss a row");

  expect(soonest.textContent).toContain(scheduleTime(lists.scheduled[0]?.scheduled_at ?? "", "en"));
  expect(
    walked.getByRole("button", { name: /^Open this draft\s*Walked along the river this morning\./ }),
  ).not.toBeNull();
  expect(first?.textContent).toContain("Saved 3h ago");
  // The marks are named for assistive technology.
  expect(walked.getByRole("img", { name: "2 files" })).not.toBeNull();
  expect(booked.getByRole("img", { name: "A reply" })).not.toBeNull();
  expect(which.getByRole("img", { name: "A poll" })).not.toBeNull();
  expect(files.getByRole("img", { name: "1 file" })).not.toBeNull();
  // A draft of files alone says it has no text.
  expect(files.getByText("No text")).not.toBeNull();
});

test("a press on a row opens its draft", async () => {
  const { user, asked, part } = list({ kind: "ready", ...lists });

  await user.click(part.getByRole("button", { name: /^Open this draft\s*Which day works/ }));

  expect(asked).toEqual(["open d3"]);
});

test("failed: a draft whose scheduled post failed says when and why, over its text", () => {
  const en = list({ kind: "ready", ...lists }).said();
  const failure = `Scheduled for ${scheduleTime(failedAt, "en")}, not posted: the post it replies to is no longer available`;
  expect(en).toContain(failure);
  expect(en.indexOf(failure)).toBeLessThan(en.indexOf("Booked two seats"));

  expect(list({ kind: "ready", ...lists }, "ja").said()).toContain(
    `${scheduleTime(failedAt, "ja")} に投稿できませんでした: 返信先の投稿が見られなくなりました`,
  );
});

test("empty: neither drafts nor scheduled posts is one sentence, with no heading of a list", () => {
  const { part } = list({ kind: "ready", drafts: [], scheduled: [] });
  expect(
    part.getByText("No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here."),
  ).not.toBeNull();
  expect(part.queryAllByRole("heading", { name: /^(Scheduled|Drafts)\s*\d/ })).toHaveLength(0);

  expect(list({ kind: "ready", drafts: [], scheduled: [] }, "ja").said()).toContain(
    "まだ下書きも予約投稿もありません。投稿を下書きに保存するか予約すると、ここに表示されます。",
  );
});

test("scheduled posts and no draft: the drafts' heading says there are none", () => {
  const { part } = list({ kind: "ready", drafts: [], scheduled: lists.scheduled });
  expect(part.getByRole("heading", { name: /^Scheduled\s*2$/ })).not.toBeNull();
  expect(part.getByRole("heading", { name: /^Drafts\s*0$/ })).not.toBeNull();
  expect(part.getByText("No drafts yet. Save a post as a draft, and it shows here.")).not.toBeNull();

  const ja = list({ kind: "ready", drafts: [], scheduled: lists.scheduled }, "ja");
  expect(ja.part.getByRole("heading", { name: /^予約\s*2$/ })).not.toBeNull();
  expect(ja.said()).toContain("まだ下書きはありません。投稿を下書きに保存すると、ここに表示されます。");
});

test("drafts and no scheduled post: the list starts at the drafts, without the zone", () => {
  const { part, said } = list({ kind: "ready", drafts: lists.drafts, scheduled: [] });

  const headings = part.getAllByRole("heading", { name: /^(Scheduled|Drafts)\s*\d/ });
  expect(headings.map((heading) => heading.textContent.replace(/\s+/g, " "))).toEqual(["Drafts 4"]);
  expect(said()).not.toContain("Times in");
});

test("loading says so; a list that could not be read says so, and Try again asks for it again", async () => {
  expect(list({ kind: "loading" }).said()).toContain("Loading drafts…");
  expect(list({ kind: "loading" }, "ja").said()).toContain("下書きを読み込んでいます…");

  const ja = list({ kind: "failed" }, "ja");
  expect(ja.said()).toContain("下書きを読み込めませんでした。");
  expect(ja.part.getByRole("button", { name: "もう一度試す" })).not.toBeNull();

  const failed = list({ kind: "failed" });
  expect(failed.said()).toContain("The drafts could not be loaded.");
  await failed.user.click(failed.part.getByRole("button", { name: "Try again" }));
  expect(failed.asked).toEqual(["retry"]);
});

test("in ja, the list's title, its headings, its zone and a row's words", () => {
  const { dialog, part, said } = list({ kind: "ready", ...lists }, "ja");

  expect(screen.getByRole("dialog", { name: "下書きと予約投稿" }) === dialog).toBe(true);
  expect(part.getByRole("heading", { name: /^予約\s*2$/ })).not.toBeNull();
  expect(part.getByRole("heading", { name: /^下書き\s*4$/ })).not.toBeNull();
  expect(said()).toContain(`${readerZone("ja")} の時刻`);
  expect(said()).toContain("3時間前に保存");
  expect(part.getAllByRole("button", { name: /^この下書きを開く/ }).length).toBeGreaterThan(0);
});

/**
 * The app's data on a server whose nearest scheduled time is `minutes` ahead, its instance read; or, without
 * `minutes`, on a server whose instance is not read yet.
 */
async function instanceWith(minutes: number | undefined) {
  const made = fakeData();
  if (minutes === undefined) return made;
  made.fake.script("GET /instance", {
    change: (instance) => ({
      ...instance,
      limits: {
        ...instance.limits,
        drafts: { ...defaultDraftLimits, scheduled_post_min_lead_minutes: minutes },
      },
    }),
  });
  await made.data.queryClient.query(made.data.queries.queryOptions("get", "/instance"));
  return made;
}

/**
 * The composer's view of `initial` in the flow of the page, as a story's specimen opens it: the view itself, which
 * the app's layer fetches as a chunk of its own.
 */
function Composing({
  initial,
  specimen = {},
  onPosted = none,
}: {
  initial: Draft;
  specimen?: ComposeSpecimen | undefined;
  onPosted?: ((posted: Draft) => void) | undefined;
}) {
  const c = useComposer(
    initial,
    {
      onPosted: (_, posted) => {
        onPosted(posted);
      },
      onKept: none,
    },
    specimen.phase,
  );
  return <ComposerView c={c} layer={false} opened={specimen.opened} onClose={none} onDismiss={none} />;
}

/** The composer of a post written as `fields` say, alone in the document, in `locale`: its dialog. */
async function composer(
  fields: Partial<Draft>,
  {
    locale = "en",
    specimen,
    target = { kind: "new" },
    onPosted,
  }: {
    locale?: string;
    specimen?: ComposeSpecimen;
    target?: Target;
    onPosted?: (posted: Draft) => void;
  } = {},
) {
  cleanup();
  const made = await instanceWith(undefined);
  const initial = { ...emptyDraft(target, defaultComposeSettings, "k"), text: "Later, maybe.", ...fields };
  const user = renderFeature(
    made.data,
    <Composing initial={initial} specimen={specimen} onPosted={onPosted} />,
    locale,
  );
  const dialog = screen.getByRole("dialog");
  // The text's editor is a chunk of its own: the composer is whole once it came.
  await within(dialog).findAllByRole("textbox");
  return { ...made, user, dialog, part: within(dialog), said: () => dialog.textContent };
}

test("the composer is named for what it writes: a new post, a reply", async () => {
  const fresh = await composer({});
  expect(screen.getByRole("dialog", { name: "New post" }) === fresh.dialog).toBe(true);

  const reply = await composer({}, { target: { kind: "reply", id: "9" } });

  expect(screen.getByRole("dialog", { name: "Reply" }) === reply.dialog).toBe(true);
});

test("a custom emoji in the text shows as its image and copies as its shortcode, and in the content warning shows as its image too", async () => {
  const { user, part, dialog } = await composer({ text: "" });
  const text = part.getByRole("textbox", { name: "Text" });
  await user.click(text);
  await pasteInto(user, text, "Tea :teacup: time\nnext line");
  // The emoji shows as its image, which has no text of its own to copy.
  expect(await within(text).findByRole("img", { name: ":teacup:" })).not.toBeNull();

  await user.keyboard("{Control>}a{/Control}");
  const copied = await user.copy();
  expect(copied?.getData("text/plain")).toBe("Tea :teacup: time\nnext line");

  await user.click(part.getByRole("button", { name: "Add a content warning" }));
  const warning = await part.findByRole("textbox", { name: "Content warning" });
  await user.click(warning);
  await pasteInto(user, warning, "Spoilers :wave:");
  expect(await within(warning).findByRole("img", { name: ":wave:" })).not.toBeNull();
  // The warning is one line: Enter writes nothing there, and the composer stays open.
  await user.keyboard("{Enter}");
  expect(warning.getAttribute("aria-multiline")).not.toBe("true");
  expect(screen.getByRole("dialog", { name: "New post" }) === dialog).toBe(true);
  expect(within(warning).queryAllByRole("img", { name: ":wave:" })).toHaveLength(1);
});

test("while a post is sent, Close stays in its place disabled and Escape leaves the composer open, and once posted it tells the screen", async () => {
  const text = "In-flight check: the ferry leaves at noon.";
  const posted: Draft[] = [];
  const { fake, user, dialog, part } = await composer({ text }, { onPosted: (sent) => posted.push(sent) });
  const sent = fake.script("POST /statuses", { hold: "response" });
  const post = part.getByRole("button", { name: "Post" });
  const close = part.getByRole<HTMLButtonElement>("button", { name: "Close" });
  expect(close.disabled).toBe(false);

  await user.click(post);
  expect((await sent.arrived()).body).toMatchObject({ text });
  // Close stays in its place, disabled, so that it cannot be pressed; Post alone shows it is sending.
  expect(part.getByRole("button", { name: "Close" }) === close).toBe(true);
  expect(close.disabled).toBe(true);
  expect(post.getAttribute("aria-busy")).toBe("true");
  await user.keyboard("{Escape}");
  expect(screen.getByRole("dialog", { name: "New post" }) === dialog).toBe(true);
  expect(posted).toHaveLength(0);

  sent.release();
  await waitFor(() => {
    if (posted.length !== 1) throw new Error("the composer did not tell the screen it posted");
  });
  expect(sent.seen).toHaveLength(1);
  // Nothing was discarded: no Undo is offered.
  expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
});

test("a new post has the drafts' menu in its head and the schedule tool off, and posts with Post", async () => {
  const { part, said } = await composer({});

  expect(part.getByRole("button", { name: "Drafts" })).not.toBeNull();
  expect(part.getByRole("button", { name: "Schedule", pressed: false })).not.toBeNull();
  expect(part.getByRole("button", { name: "Post" })).not.toBeNull();
  expect(said()).not.toMatch(/Scheduled for .+Change time/);
});

test("scheduled: the tool is on, the time line says when in the zone, Change time at its end, and the button reads Schedule", async () => {
  const at = from(day);
  const { part, said } = await composer({ scheduledAt: at });

  expect(part.getByRole("button", { name: "Change the scheduled time", pressed: true })).not.toBeNull();
  // The time line states the zone, the form being closed; Change time follows it with no dot.
  expect(said()).toContain(`Scheduled for ${scheduleTime(at, "en")} (${readerZone("en")})Change time`);
  expect(part.getByRole("button", { name: "Change time" })).not.toBeNull();
  // Schedule takes Post's place.
  expect(part.getByRole("button", { name: "Schedule" })).not.toBeNull();
  expect(part.queryAllByRole("button", { name: "Post" })).toHaveLength(0);

  const ja = await composer({ scheduledAt: at }, { locale: "ja" });
  expect(ja.said()).toContain(`${scheduleTime(at, "ja")} (${readerZone("ja")}) に投稿予定時刻を変更`);
  expect(ja.part.getByRole("button", { name: "予約する" })).not.toBeNull();
});

function Scheduling({ at, locale }: { at: string | undefined; locale: string }) {
  const settings = useComposeSettings();
  const { t } = stringsIn(locale);
  return <ScheduleTimeForm at={at} limits={settings.schedule} onSet={none} onDone={none} t={t} locale={locale} />;
}

/**
 * The schedule's form from `at`, alone in the document, with the server's limits as the composer reads them (the
 * nearest `minutes` ahead): what it says.
 */
async function scheduleForm(
  at: string | undefined,
  { locale = "en", minutes }: { locale?: string; minutes?: number } = {},
) {
  cleanup();
  const { data } = await instanceWith(minutes);
  const { container } =
    (renderFeature(data, <Scheduling at={at} locale={locale} />, locale), { container: document.body });
  return container.textContent;
}

test("the schedule's form: the time in the reader's zone, the server's limits, and Set time", async () => {
  const said = await scheduleForm(undefined);

  expect(screen.getByLabelText<HTMLInputElement>("Date and time").type).toBe("datetime-local");
  expect(said).toContain(readerZone("en"));
  expect(said).toContain("From 5 minutes to 366 days from now");
  expect(screen.getByRole("button", { name: "Set time" })).not.toBeNull();
  // Nothing to take back while no time is set.
  expect(screen.queryAllByRole("button", { name: "Don't schedule" })).toHaveLength(0);

  await scheduleForm(from(day));
  expect(screen.getByRole("button", { name: "Don't schedule" })).not.toBeNull();
});

test("a time nearer than the server's nearest, or farther than its farthest, says so in the form, and the field is invalid", async () => {
  expect(await scheduleForm(from(2 * 60_000))).toContain("Schedule the post at least 5 minutes from now.");
  expect(screen.getByLabelText("Date and time").getAttribute("aria-invalid")).toBe("true");
  expect(await scheduleForm(from(400 * day))).toContain("Schedule the post at most 366 days from now.");
  expect(await scheduleForm(from(2 * 60_000), { locale: "ja" })).toContain(
    "予約する時刻は、今から 5 分以上先にしてください。",
  );
});

test("the form takes the instance's limits once they are read", async () => {
  const said = await scheduleForm(from(2 * 60_000), { minutes: 1 });

  expect(said).toContain("From 1 minute to 366 days from now");
  expect(said).not.toContain("Schedule the post at least");
});

test("failed: a draft whose scheduled post failed says why and what to do next", async () => {
  const en = await composer({ failure: { publishAt: failedAt, reason: "reply_unavailable" } });
  expect(en.said()).toContain(
    `Scheduled for ${scheduleTime(failedAt, "en")}, not posted: the post it replies to is no longer available. Edit it if needed, then post it or schedule it again.`,
  );

  const ja = await composer({ failure: { publishAt: failedAt, reason: "rejected" } }, { locale: "ja" });
  expect(ja.said()).toContain(
    `${scheduleTime(failedAt, "ja")} に投稿できませんでした: サーバーの自動審査で受け付けられませんでした。必要なら直してから、投稿するか予約し直してください。`,
  );
});

test("a save refused for a newer draft saved elsewhere says the changes were not saved, and offers the newer one", async () => {
  const { part } = await composer({}, { specimen: { phase: { kind: "conflict" } } });
  const alert = part.getByRole("alert");

  expect(alert.textContent).toMatch(
    /^This draft was saved later on another device, so your changes here were not saved\.\s*Open the newer one$/,
  );
  expect(within(alert).getByRole("button", { name: "Open the newer one" })).not.toBeNull();

  const ja = await composer({}, { specimen: { phase: { kind: "conflict" } }, locale: "ja" });
  expect(ja.part.getByRole("alert").textContent).toContain("ここでの変更は保存されませんでした。");
  expect(within(ja.part.getByRole("alert")).getByRole("button", { name: "新しいほうを開く" })).not.toBeNull();
});

test("a draft gone from the server says it may have been posted, keeps what was written, and Post reads Post as new", async () => {
  const { part, said } = await composer({}, { specimen: { phase: { kind: "gone" } } });
  const alert = part.getByRole("alert");

  expect(alert.textContent).toBe("This draft is no longer on the server. It may have been posted; check your profile.");
  // One action: Post itself, which reads Post as new; the alert offers none of its own.
  expect(within(alert).queryAllByRole("button")).toHaveLength(0);
  expect(part.getAllByRole("button", { name: "Post as new" })).toHaveLength(1);
  expect(part.queryAllByRole("button", { name: "Post" })).toHaveLength(0);
  expect(follows(alert, part.getByRole("button", { name: "Post as new" }))).toBe(true);
  // The text stays: its count is the text's.
  expect(said()).toContain(`${String(500 - "Later, maybe.".length)} characters left`);

  const ja = await composer({}, { specimen: { phase: { kind: "gone" } }, locale: "ja" });
  expect(ja.part.getByRole("alert").textContent).toBe(
    "この下書きはもうサーバーにありません。投稿済みかもしれないので、プロフィールで確かめてください。",
  );
  expect(ja.part.getByRole("button", { name: "新しく投稿する" })).not.toBeNull();
});

test("Post as new lets the gone draft go and posts what is written by POST /statuses, under a new key", async () => {
  let heard: (posted: Draft) => void = none;
  const posted = new Promise<Draft>((resolve) => {
    heard = resolve;
  });
  const { fake, user, part, requests } = await composer(
    { saved: { id: "d1", etag: '"1"' }, stored: true, scheduledAt: from(day) },
    {
      specimen: { phase: { kind: "gone" } },
      onPosted: (draft) => {
        heard(draft);
      },
    },
  );
  const sent = fake.script("POST /statuses");

  await user.click(part.getByRole("button", { name: "Post as new" }));

  expect((await sent.arrived()).body).toMatchObject({ text: "Later, maybe." });
  const d = await posted;
  // Nothing of the gone draft is written again: no save over it, no publication of it, no schedule.
  expect(requests.filter((r) => r.includes("/drafts"))).toEqual([]);
  expect(d.saved).toBeUndefined();
  expect(d.scheduledAt).toBeUndefined();
  expect(d.key).not.toBe("k");
});

test("a draft's image whose file could not be read says so, and offers to load it again", async () => {
  const failed = { attachments: [{ key: "draft-m1", id: "m1", state: "failed" as const, description: "" }] };
  const { part, said } = await composer(failed);
  const status = part.getByText("The image could not be loaded.").closest<HTMLElement>('[role="status"]');
  if (status === null) throw new Error("that the image could not be loaded is no status");

  expect(within(status).getByRole("button", { name: "Try again" })).not.toBeNull();
  expect(said()).not.toContain("The upload did not finish.");

  expect((await composer(failed, { locale: "ja" })).said()).toContain("画像を読み込めませんでした。");
});

test("an edit of a posted post is no draft: no drafts' menu, no schedule tool", async () => {
  const { part } = await composer(
    {},
    {
      target: {
        kind: "edit",
        id: "p1",
        source: {
          id: "p1",
          text: "Posted.",
          spoiler_text: "",
          sensitive: false,
          visibility: "public",
          language: "en",
          media: [],
        },
      } as unknown as Target,
    },
  );

  expect(part.queryAllByRole("button", { name: /^(Drafts|Schedule)$/ })).toHaveLength(0);
});

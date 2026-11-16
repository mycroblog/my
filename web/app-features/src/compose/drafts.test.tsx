// SPDX-License-Identifier: AGPL-3.0-only

import { expect, test } from "bun:test";
import { defaultComposeSettings, defaultDraftLimits, type ServerDraft } from "@my/app-data";
import { readerZone, scheduleTime } from "@my/design-system";
import { render, server, status } from "../test/server";
import { ComposerView } from "./composer";
import { DraftsView, type DraftsState } from "./drafts";
import type { ComposeSpecimen } from "./layer";
import { emptyDraft, type Draft, type Target } from "./model";
import { useComposer } from "./use-composer";

const hour = 3_600_000;
const day = 24 * hour;

/** A moment `ms` from now, as the server writes it. */
const from = (ms: number) => new Date(Date.now() + ms).toISOString();

/** The words of markup, as a reader reads them: without its tags, its entities decoded. */
const words = (html: string) =>
  html
    .replace(/<[^>]+>/g, "")
    .replaceAll("&#x27;", "'")
    .replaceAll("&quot;", '"')
    .replaceAll("&lt;", "<")
    .replaceAll("&gt;", ">")
    .replaceAll("&amp;", "&");

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

/** The drafts' list in `state`, in `locale`. */
function list(state: DraftsState, locale = "en") {
  const { data } = server({});
  return render(
    data,
    <DraftsView
      state={state}
      layer={false}
      phone="dialog"
      onBack={none}
      onClose={none}
      onDismiss={none}
      onRetry={none}
      onOpen={none}
      onUnschedule={none}
      onDelete={none}
    />,
    locale,
  );
}

test("listed: the scheduled posts first, under one line of the reader's zone, then the drafts", () => {
  const html = list({ kind: "ready", ...lists });
  const text = words(html);
  expect(text).toContain("Drafts and scheduled posts");
  expect(text).toContain("Back to the post");
  const scheduled = text.indexOf("Scheduled 2");
  const drafts = text.indexOf("Drafts 4");
  expect(scheduled).toBeGreaterThan(-1);
  expect(drafts).toBeGreaterThan(scheduled);
  // One line of the zone, over the scheduled posts alone.
  expect(text.split(`Times in ${readerZone("en")}`)).toHaveLength(2);
  expect(text.indexOf(`Times in ${readerZone("en")}`)).toBeLessThan(drafts);
  // Each list is named, the rows in the order given: the scheduled soonest first, the drafts newest first.
  expect(html).toContain('aria-label="Scheduled posts"');
  expect(html).toContain('aria-label="Drafts"');
  const order = ["The drawing session", "Weekly photo thread", "Walked along", "Booked two", "Which day"];
  expect(order.map((s) => text.indexOf(s))).toEqual(order.map((s) => text.indexOf(s)).toSorted((a, b) => a - b));
});

test("a scheduled post reads by the time it goes out; a draft by what it carries and when it was saved", () => {
  const text = words(list({ kind: "ready", ...lists }));
  expect(text).toContain(scheduleTime(lists.scheduled[0]?.scheduled_at ?? "", "en"));
  expect(text).toContain("Open this draft");
  expect(text).toContain("Saved 3h ago");
  // The marks are named for assistive technology.
  const html = list({ kind: "ready", ...lists });
  for (const mark of ["2 files", "A reply", "A poll", "1 file"]) expect(html).toContain(`aria-label="${mark}"`);
  // A draft of files alone says it has no text.
  expect(text).toContain("No text");
});

test("failed: a draft whose scheduled post failed says when and why, over its text", () => {
  const text = words(list({ kind: "ready", ...lists }));
  const failure = `Scheduled for ${scheduleTime(failedAt, "en")}, not posted: the post it replies to is no longer available`;
  expect(text).toContain(failure);
  expect(text.indexOf(failure)).toBeLessThan(text.indexOf("Booked two seats"));
  const ja = words(list({ kind: "ready", ...lists }, "ja"));
  expect(ja).toContain(`${scheduleTime(failedAt, "ja")} に投稿できませんでした: 返信先の投稿が見られなくなりました`);
});

test("empty: neither drafts nor scheduled posts is one sentence, with no heading", () => {
  const text = words(list({ kind: "ready", drafts: [], scheduled: [] }));
  expect(text).toContain("No drafts or scheduled posts yet. Save a post as a draft or schedule it, and it shows here.");
  expect(text).not.toContain("Scheduled 0");
  expect(text).not.toContain("Drafts 0");
  expect(words(list({ kind: "ready", drafts: [], scheduled: [] }, "ja"))).toContain(
    "まだ下書きも予約投稿もありません。投稿を下書きに保存するか予約すると、ここに表示されます。",
  );
});

test("scheduled posts and no draft: the drafts' heading says there are none", () => {
  const text = words(list({ kind: "ready", drafts: [], scheduled: lists.scheduled }));
  expect(text).toContain("Scheduled 2");
  expect(text).toContain("Drafts 0");
  expect(text).toContain("No drafts yet. Save a post as a draft, and it shows here.");
  const ja = words(list({ kind: "ready", drafts: [], scheduled: lists.scheduled }, "ja"));
  expect(ja).toContain("予約 2");
  expect(ja).toContain("まだ下書きはありません。投稿を下書きに保存すると、ここに表示されます。");
});

test("drafts and no scheduled post: the list starts at the drafts, without the zone", () => {
  const text = words(list({ kind: "ready", drafts: lists.drafts, scheduled: [] }));
  expect(text).not.toMatch(/Scheduled \d/);
  expect(text).not.toContain("Times in");
  expect(text).toContain("Drafts 4");
});

test("loading, and a list that could not be read with Try again", () => {
  expect(words(list({ kind: "loading" }))).toContain("Loading drafts…");
  const failed = words(list({ kind: "failed" }));
  expect(failed).toContain("The drafts could not be loaded.");
  expect(failed).toContain("Try again");
  expect(words(list({ kind: "loading" }, "ja"))).toContain("下書きを読み込んでいます…");
  const ja = words(list({ kind: "failed" }, "ja"));
  expect(ja).toContain("下書きを読み込めませんでした。");
  expect(ja).toContain("もう一度試す");
});

test("in ja, the list's headings, its zone and a row's saved time", () => {
  const ja = words(list({ kind: "ready", ...lists }, "ja"));
  expect(ja).toContain("下書きと予約投稿");
  expect(ja).toContain("予約 2");
  expect(ja).toContain("下書き 4");
  expect(ja).toContain(`${readerZone("ja")} の時刻`);
  expect(ja).toContain("3時間前に保存");
  expect(ja).toContain("この下書きを開く");
});

/** The composer of a new post written as `fields` say, in `locale`, under the instance's `limits` once read. */
async function composer(
  fields: Partial<Draft>,
  {
    locale = "en",
    specimen,
    target = { kind: "new" },
    minutes,
  }: {
    locale?: string;
    specimen?: ComposeSpecimen;
    target?: Target;
    minutes?: number;
  } = {},
) {
  const { data } = server({
    "/instance": {
      domain: "my.example",
      limits: {
        max_post_characters: 500,
        drafts: { ...defaultDraftLimits, scheduled_post_min_lead_minutes: minutes ?? 5 },
      },
    },
  });
  if (minutes !== undefined) await data.queryClient.query(data.queries.queryOptions("get", "/instance"));
  const initial = { ...emptyDraft(target, defaultComposeSettings, "k"), text: "Later, maybe.", ...fields };
  return render(data, <Composing initial={initial} specimen={specimen} />, locale);
}

/**
 * The composer's view of `initial` in the flow of the page, as a story's specimen opens it: the view itself, which the
 * app's layer fetches as a chunk of its own, and which a server render would leave out.
 */
function Composing({ initial, specimen = {} }: { initial: Draft; specimen?: ComposeSpecimen | undefined }) {
  const c = useComposer(initial, { onPosted: none, onKept: none }, specimen.phase);
  return <ComposerView c={c} layer={false} opened={specimen.opened} onClose={none} onDismiss={none} />;
}

test("a new post has the drafts' menu in its head and the schedule tool off, and posts with Post", async () => {
  const html = await composer({});
  expect(html).toMatch(/aria-label="Drafts"/);
  expect(html).toMatch(/aria-label="Schedule"[^>]*aria-pressed="false"|aria-pressed="false"[^>]*aria-label="Schedule"/);
  expect(html).toContain(">Post<");
  expect(words(html)).not.toMatch(/Scheduled for .+Change time/);
});

test("scheduled: the tool is on, the time line says when in the zone, Change at its end, and the button reads Schedule", async () => {
  const at = from(day);
  const html = await composer({ scheduledAt: at });
  expect(html).toMatch(
    /aria-label="Change the scheduled time"[^>]*aria-pressed="true"|aria-pressed="true"[^>]*aria-label="Change the scheduled time"/,
  );
  const text = words(html);
  // The time line states the zone, the form being closed; Change time follows it with no dot.
  expect(text).toContain(`Scheduled for ${scheduleTime(at, "en")} (${readerZone("en")})Change time`);
  // Schedule takes Post's place.
  expect(html).toContain(">Schedule<");
  expect(html).not.toContain(">Post<");
  const ja = words(await composer({ scheduledAt: at }, { locale: "ja" }));
  expect(ja).toContain(`${scheduleTime(at, "ja")} (${readerZone("ja")}) に投稿予定時刻を変更`);
  expect(ja).toContain("予約する");
});

test("the schedule's form: the time in the reader's zone, the server's limits, and Set time", async () => {
  const html = await composer({}, { specimen: { opened: "schedule" } });
  expect(html).toMatch(/role="dialog"[^>]*aria-label="Schedule"|aria-label="Schedule"[^>]*role="dialog"/);
  expect(html).toMatch(/type="datetime-local"[^>]*aria-label="Date and time"/);
  const text = words(html);
  expect(text).toContain(readerZone("en"));
  expect(text).toContain("From 5 minutes to 366 days from now");
  expect(text).toContain("Set time");
  // Nothing to take back while no time is set.
  expect(text).not.toContain("Don't schedule");
  const set = words(await composer({ scheduledAt: from(day) }, { specimen: { opened: "schedule" } }));
  expect(set).toContain("Don't schedule");
});

test("a time nearer than the server's nearest, or farther than its farthest, says so in the form", async () => {
  const soon = words(await composer({ scheduledAt: from(2 * 60_000) }, { specimen: { opened: "schedule" } }));
  expect(soon).toContain("Schedule the post at least 5 minutes from now.");
  const far = words(await composer({ scheduledAt: from(400 * day) }, { specimen: { opened: "schedule" } }));
  expect(far).toContain("Schedule the post at most 366 days from now.");
  const ja = words(
    await composer({ scheduledAt: from(2 * 60_000) }, { specimen: { opened: "schedule" }, locale: "ja" }),
  );
  expect(ja).toContain("予約する時刻は、今から 5 分以上先にしてください。");
});

test("the form takes the instance's limits once they are read", async () => {
  const html = words(
    await composer({ scheduledAt: from(2 * 60_000) }, { specimen: { opened: "schedule" }, minutes: 1 }),
  );
  expect(html).toContain("From 1 minute to 366 days from now");
  expect(html).not.toContain("Schedule the post at least");
});

test("failed: a draft whose scheduled post failed says why and what to do next", async () => {
  const text = words(await composer({ failure: { publishAt: failedAt, reason: "reply_unavailable" } }));
  expect(text).toContain(
    `Scheduled for ${scheduleTime(failedAt, "en")}, not posted: the post it replies to is no longer available. Edit it if needed, then post it or schedule it again.`,
  );
  const ja = words(await composer({ failure: { publishAt: failedAt, reason: "rejected" } }, { locale: "ja" }));
  expect(ja).toContain(
    `${scheduleTime(failedAt, "ja")} に投稿できませんでした: サーバーの自動審査で受け付けられませんでした。必要なら直してから、投稿するか予約し直してください。`,
  );
});

test("a save refused for a newer draft saved elsewhere says the changes were not saved, and offers the newer one", async () => {
  const html = await composer({}, { specimen: { phase: { kind: "conflict" } } });
  expect(html).toContain('role="alert"');
  const text = words(html);
  expect(text).toContain(
    "This draft was saved later on another device, so your changes here were not saved. Open the newer one",
  );
  const ja = words(await composer({}, { specimen: { phase: { kind: "conflict" } }, locale: "ja" }));
  expect(ja).toContain("ここでの変更は保存されませんでした。");
  expect(ja).toContain("新しいほうを開く");
});

test("a draft gone from the server says it may have been posted, keeps what was written, and Post reads Post as new", async () => {
  const html = await composer({}, { specimen: { phase: { kind: "gone" } } });
  expect(html).toContain('role="alert"');
  const text = words(html);
  expect(text).toContain("This draft is no longer on the server. It may have been posted; check your profile.");
  // One action: Post itself, which reads Post as new; the alert offers none of its own.
  expect(html.match(/>Post as new</g)).toHaveLength(1);
  expect(html).not.toContain(">Post<");
  expect(html.indexOf('role="alert"')).toBeLessThan(html.indexOf(">Post as new<"));
  expect(html.slice(html.indexOf('role="alert"'), html.indexOf("</p>", html.indexOf('role="alert"')))).not.toContain(
    "<button",
  );
  // The text stays (its editor, a chunk of its own, is left out of a server render): its count is the text's.
  expect(text).toContain(`${String(500 - "Later, maybe.".length)} characters left`);
  const ja = await composer({}, { specimen: { phase: { kind: "gone" } }, locale: "ja" });
  expect(words(ja)).toContain(
    "この下書きはもうサーバーにありません。投稿済みかもしれないので、プロフィールで確かめてください。",
  );
  expect(ja).toContain(">新しく投稿する<");
});

test("Post as new lets the gone draft go and posts what is written by POST /statuses, under a new key", async () => {
  const { data, requests, received } = server({
    "/statuses": status("s1", "lena", { content: "<p>Later, maybe.</p>" }),
  });
  const initial: Draft = {
    ...emptyDraft({ kind: "new" }, defaultComposeSettings, "k"),
    text: "Later, maybe.",
    saved: { id: "d1", etag: '"1"' },
    stored: true,
    scheduledAt: from(day),
  };
  const seen: { c?: ReturnType<typeof useComposer> } = {};
  let heard: (d: Draft) => void = none;
  const posted = new Promise<Draft>((resolve) => {
    heard = resolve;
  });
  function Probe() {
    seen.c = useComposer(
      initial,
      {
        onPosted: (_, d) => {
          heard(d);
        },
        onKept: none,
      },
      { kind: "gone" },
    );
    return null;
  }
  render(data, <Probe />);
  seen.c?.submit();
  expect(await received("POST /statuses")).toMatchObject({ text: "Later, maybe." });
  const d = await posted;
  // Nothing of the gone draft is written again: no save over it, no publication of it, no schedule.
  expect(requests.filter((r) => r.includes("/drafts"))).toEqual([]);
  expect(d.saved).toBeUndefined();
  expect(d.scheduledAt).toBeUndefined();
  expect(d.key).not.toBe("k");
});

test("a draft's image whose file could not be read says so, and offers to load it again", async () => {
  const text = words(
    await composer({ attachments: [{ key: "draft-m1", id: "m1", state: "failed", description: "" }] }),
  );
  expect(text).toContain("The image could not be loaded.");
  expect(text).toContain("Try again");
  expect(text).not.toContain("The upload did not finish.");
  const ja = words(
    await composer(
      { attachments: [{ key: "draft-m1", id: "m1", state: "failed", description: "" }] },
      { locale: "ja" },
    ),
  );
  expect(ja).toContain("画像を読み込めませんでした。");
});

test("an edit of a posted post is no draft: no drafts' menu, no schedule tool", async () => {
  const html = await composer(
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
  expect(html).not.toContain('aria-label="Drafts"');
  expect(html).not.toContain('aria-label="Schedule"');
});

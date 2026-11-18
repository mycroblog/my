// SPDX-License-Identifier: AGPL-3.0-only

// The reader's drafts and scheduled posts of @my/app-features, in the composer's frame, which the composer's menu
// opens: a story per state of the list, in the flow of the page. The language is the toolbar's locale (ja, else en);
// the times read in the browser's zone.

import type { ServerDraft } from "@my/app-data";
import { DraftsView, type DraftsState } from "@my/app-features";
import { instantAt, useLocale, wallTime } from "@my/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useNarrowViewport } from "./shell/viewport";

/** A moment `hours` hours from now, as an ISO 8601 string. */
const hoursFrom = (hours: number) => new Date(Date.now() + hours * 3_600_000).toISOString();
/** A moment `days` days from now at `hour` o'clock, local time. */
const dayAt = (days: number, hour: number, minute = 0) => {
  const today = wallTime(new Date(), undefined);
  return instantAt({ ...today, day: today.day + days, hour, minute }, undefined).toISOString();
};

const words = {
  en: {
    river:
      "Walked along the river this morning. The first frost turned every leaf on the path silver, and the herons were back on the old pier.",
    meetup: "Which day works for the next meetup?",
    bread: "@sam The sourdough came out flat again. Trying your overnight proof this weekend.",
    cw: "Spoilers for the season finale",
    finale: "I did not expect the lighthouse keeper to be the narrator all along.",
    rules:
      "Server notes for November: we are moving the backups to a second drive, and the rules page gets the two changes we talked about at the meetup.",
    weekly: "Weekly photo thread is open. Post one picture from your week, no captions needed.",
    reminder: "Reminder: the drawing session is tomorrow at 2 pm, second floor of the community hall.",
  },
  ja: {
    river: "今朝は川沿いを歩いた。初霜で道の葉っぱがぜんぶ銀色になっていて、古い桟橋にはサギが戻ってきていた。",
    meetup: "次のオフ会、どの日がいい？",
    bread: "@sam サワードウがまたぺたんこに。週末は教わった一晩発酵でやってみます。",
    cw: "最終回のネタバレ",
    finale: "灯台守がずっと語り手だったとは思わなかった。",
    rules:
      "11 月のサーバーのお知らせ: バックアップを二台目のディスクに移します。ルールのページには、オフ会で話した二つの変更を入れます。",
    weekly: "今週の写真スレッドを開けました。一週間の一枚を貼ってください。説明はなくて大丈夫。",
    reminder: "お知らせ: 絵を描く会は明日の午後二時から、公民館の二階です。",
  },
};

const media = (n: number) =>
  Array.from({ length: n }, (_, i) => ({ id: `media-${String(i)}` })) as unknown as ServerDraft["media"];

/** A draft of the reader's, saved `savedHours` ago. */
const draft = (id: string, text: string, savedHours: number, more: Partial<ServerDraft> = {}): ServerDraft => ({
  id,
  created_at: hoursFrom(-savedHours - 1),
  updated_at: hoursFrom(-savedHours),
  text,
  spoiler_text: "",
  sensitive: false,
  visibility: "public",
  media: [],
  ...more,
});

/** The drafts, newest first, and the scheduled posts, soonest first. */
function samples(w: (typeof words)["en"]) {
  return {
    drafts: [
      draft("d1", w.river, 0.2, { media: media(2) }),
      draft("d2", w.bread, 5, {
        in_reply_to_id: "s1",
        last_schedule: {
          publish_at: dayAt(-1, 9),
          outcome: "failed",
          reason: "reply_unavailable",
          occurred_at: dayAt(-1, 9),
        },
      }),
      draft("d3", w.meetup, 26, { poll: { options: ["", ""], expires_in: 86400 } }),
      draft("d4", w.finale, 72, { spoiler_text: w.cw }),
      draft("d5", "", 24 * 9, { media: media(1) }),
    ],
    scheduled: [
      draft("s1", w.reminder, 3, { scheduled_at: dayAt(1, 9) }),
      draft("s2", w.weekly, 30, { scheduled_at: dayAt(4, 12, 30), media: media(1) }),
      draft("s3", w.rules, 48, { scheduled_at: dayAt(17, 20) }),
    ],
  };
}

type Kind = "listed" | "empty" | "empty-drafts" | "empty-scheduled" | "loading" | "failed";

function stateOf(kind: Kind, w: (typeof words)["en"]): DraftsState {
  const { drafts, scheduled } = samples(w);
  switch (kind) {
    case "loading":
      return { kind: "loading" };
    case "failed":
      return { kind: "failed" };
    case "empty":
      return { kind: "ready", drafts: [], scheduled: [] };
    case "empty-scheduled":
      return { kind: "ready", drafts, scheduled: [] };
    case "empty-drafts":
      return { kind: "ready", drafts: [], scheduled };
    default:
      return { kind: "ready", drafts, scheduled };
  }
}

function Specimen({ kind }: { kind: Kind }) {
  const { locale } = useLocale();
  const narrow = useNarrowViewport();
  const w = words[locale.startsWith("ja") ? "ja" : "en"];
  const none = () => undefined;
  return (
    <div style={{ maxInlineSize: "37.5rem", marginInline: "auto" }}>
      <DraftsView
        state={stateOf(kind, w)}
        layer={false}
        phone={narrow ? "screen" : "dialog"}
        onBack={none}
        onClose={none}
        onDismiss={none}
        onRetry={none}
        onOpen={none}
        onUnschedule={none}
        onDelete={none}
      />
    </div>
  );
}

const meta = {
  title: "Drafts",
  tags: ["themes"],
  parameters: { screen: true, layout: "fullscreen" },
  render: (_, { id, parameters }) => <Specimen key={id} kind={(parameters as { drafts?: Kind }).drafts ?? "listed"} />,
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const shows = (kind: Kind): Story => ({ parameters: { drafts: kind } });

/**
 * One list: the scheduled posts first, soonest first, by the time each goes out, under the one line of the zone; then
 * the drafts, newest first: the start of the text after its warning, what it carries, and when it was saved; one
 * whose scheduled post failed says why over its text.
 */
export const Listed: Story = shows("listed");
/** Scheduled posts and no draft: the drafts' heading says so under it. */
export const NoDrafts: Story = shows("empty-drafts");
/** Drafts and no scheduled post: the list starts at the drafts. */
export const NoneScheduled: Story = shows("empty-scheduled");
/** Neither: one muted sentence where the first heading would stand. */
export const Empty: Story = shows("empty");
export const Loading: Story = shows("loading");
export const LoadFailed: Story = shows("failed");

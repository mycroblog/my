// SPDX-License-Identifier: AGPL-3.0-only

// Asking for the author's approval to reply, boost and quote (ADR-0155, #3113), played on the fake server's posts that
// approve by hand (`approvals`), in en and in ja: the composer's choice of who may reply, boost and quote, with others
// asking for approval, and what says that a reply or a boost will wait; what waits for the reader's approval, counted
// over the notifications and approved or declined on its screen; and the lines over the reader's reply and boost that
// wait or were not approved, which no one else sees.
//
// The fake's reader is yuki: marco asks to reply to and boost yuki-asks and mio to quote it; layla-asks admits replies
// and boosts of the people it mentions at once and of everyone else by approval, and holds yuki's reply that waits,
// yuki's reply that was not approved, and yuki's boost that waits. A play that opens the composer ends once its dialog
// is at rest, so that the story tests check its colors as the reader sees them, not halfway through its entrance.

import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { minimumWidth, screen, screenMeta } from "./app.samples";

const meta = {
  title: "Approval states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/notifications", server: { approvals: true } }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A name that holds a person's name, which the app isolates (U+2068…U+2069), whole. */
const named = (before: string, person: string, after: string) => new RegExp(`^${before}⁨?${person}⁩?${after}$`);

/** The words of the screens the plays read, in each locale they run in. */
const words = {
  en: {
    notifications: "Notifications",
    all: "All",
    mentions: "Mentions",
    waiting: "Waiting for approval",
    waitingCount: (n: number) => `${String(n)} waiting for your approval`,
    askedReply: named("", "Marco", " asked to reply to your post"),
    askedBoost: named("", "Marco", " asked to boost your post"),
    askedQuote: named("", "Mio 澪", " asked to quote your post"),
    approve: "Approve",
    decline: "Decline",
    accepted: "Approved",
    declined: "Declined",
    reply: /^Reply(, \d+)?$/,
    boost: /^Boost(, \d+)?$/,
    boostMenu: "Boost",
    replyDialog: "Reply",
    replyWaits: named("Others see your reply once ", "@layla", " approves it"),
    approval: /approv/i,
    audienceMenu: "Who can see it",
    audience: /^Who can see it: Public(, replies, boosts or quotes limited)?$/,
    limited: "Who can see it: Public, replies, boosts or quotes limited",
    permissions: "Who can reply, boost and quote",
    kind: { reply: "Who can reply", announce: "Who can boost", quote: "Who can quote" },
    everyone: "Anyone",
    followers: "Followers",
    nobody: "Only you",
    asks: "Allow others with your approval",
    withApproval: (audience: string) => `${audience}, others with your approval`,
    pending: "Waiting for approval",
    rejected: "Not approved",
    boostWaits: "Boost waiting for approval",
    noReplies: "No replies yet.",
  },
  ja: {
    notifications: "通知",
    all: "すべて",
    mentions: "メンション",
    waiting: "承認待ち",
    waitingCount: (n: number) => `承認待ち ${String(n)}件`,
    askedReply: named("", "Marco", "さんが返信の承認を求めています"),
    askedBoost: named("", "Marco", "さんがブーストの承認を求めています"),
    askedQuote: named("", "Mio 澪", "さんが引用の承認を求めています"),
    approve: "承認",
    decline: "断る",
    accepted: "承認しました",
    declined: "断りました",
    reply: /^返信(、\d+)?$/,
    boost: /^ブースト(、\d+)?$/,
    boostMenu: "ブースト",
    replyDialog: "返信",
    replyWaits: named("", "@layla", "さんが承認すると、ほかの人にも表示されます"),
    approval: /承認/,
    audienceMenu: "見られる人",
    audience: /^見られる人: 公開(、返信・ブースト・引用を制限中)?$/,
    limited: "見られる人: 公開、返信・ブースト・引用を制限中",
    permissions: "返信・ブースト・引用できる人",
    kind: { reply: "返信できる人", announce: "ブーストできる人", quote: "引用できる人" },
    everyone: "誰でも",
    followers: "フォロワー",
    nobody: "自分だけ",
    asks: "ほかの人も、承認すればできるようにする",
    withApproval: (audience: string) => `${audience}、ほかの人は承認すれば`,
    pending: "承認待ち",
    rejected: "承認されませんでした",
    boostWaits: "ブーストの承認待ち",
    noReplies: "まだ返信はありません。",
  },
};

type Lang = keyof typeof words;
type Words = (typeof words)[Lang];
type Kind = keyof Words["kind"];

const pc = { viewport: { value: "pc" } };

/** The screen in the canvas once it shows its state: its main region, the page's body (for popovers), the words. */
async function settled(context: StoryContext, lang: Lang) {
  await screenMeta.play(context);
  const root = context.canvasElement;
  return { w: words[lang], main: within(within(root).getByRole("main")), body: within(root.ownerDocument.body) };
}

type Queries = Awaited<ReturnType<typeof settled>>["main"];

/** Waits until nothing on the page moves: the composer's entrance, a menu's, a panel's slide. */
const atRest = (root: HTMLElement) =>
  waitFor(() => expect(root.ownerDocument.getAnimations().filter((a) => a.playState === "running")).toHaveLength(0));

/** The row over the notifications that counts what waits, a link. */
const waitingRow = (main: Queries, w: Words, count: number) => main.findByRole("link", { name: w.waitingCount(count) });

/** The article of the screen's post whose text holds `text`. */
async function postOf(main: Queries, text: string): Promise<HTMLElement> {
  const found = (await main.findAllByRole("article")).find((a) => a.textContent.includes(text));
  if (found === undefined) throw new Error(`no post holds ${text}`);
  return found;
}

/**
 * Over the notifications, in both tabs, a row counts the three requests and opens their screen; the requests'
 * notifications are lines without buttons.
 */
async function counted(context: StoryContext, lang: Lang) {
  const { w, main } = await settled(context, lang);
  const feed = within(await main.findByRole("feed", { name: w.notifications }));
  const row = await waitingRow(main, w, 3);
  const [first] = feed.getAllByRole("article");
  if (first === undefined) throw new Error("the notifications show none");
  await expect(row.compareDocumentPosition(first) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  for (const asked of [w.askedReply, w.askedBoost, w.askedQuote])
    await expect(within(await feed.findByRole("article", { name: asked })).queryAllByRole("button")).toHaveLength(0);
  await userEvent.click(main.getByRole("tab", { name: w.mentions }));
  await waitingRow(main, w, 3);
  await userEvent.click(main.getByRole("tab", { name: w.all }));
  await userEvent.click(await waitingRow(main, w, 3));
  await expect(await main.findByRole("feed", { name: w.waiting })).toBeVisible();
}

export const Counted: Story = { globals: { locale: "en" }, play: async (c) => counted(c, "en") };
export const CountedJapanesePC: Story = { globals: { locale: "ja", ...pc }, play: async (c) => counted(c, "ja") };

const laylaAsks = screen({ at: "layla-asks", server: { approvals: true } });
const laylaAsksToSam = screen({ at: "layla-asks", viewer: "sam", server: { approvals: true } });

/** Under layla-asks, the reader's waiting boost and replies each carry their line. */
async function ownLines(context: StoryContext, lang: Lang) {
  const { w, main } = await settled(context, lang);
  await waitFor(async () => expect(await postOf(main, "Translating a poem")).toHaveTextContent(w.boostWaits));
  await expect(await postOf(main, "どの詩ですか")).toHaveTextContent(w.pending);
  await expect(await postOf(main, "翻訳ができたら")).toHaveTextContent(w.rejected);
}

export const OwnLines: Story = {
  globals: { locale: "en" },
  parameters: laylaAsks,
  play: async (c) => ownLines(c, "en"),
};
export const OwnLinesJapaneseMinimumWidth: Story = {
  globals: { locale: "ja", ...minimumWidth },
  parameters: laylaAsks,
  play: async (c) => ownLines(c, "ja"),
};

/** Someone else reads layla-asks: neither yuki's reply that waits nor the one declined, and no line. */
async function othersSee(context: StoryContext, lang: Lang) {
  const { w, main } = await settled(context, lang);
  await expect(await main.findByText(w.noReplies)).toBeVisible();
  await expect(main.queryByText(/どの詩ですか|翻訳ができたら/)).toBeNull();
  await expect(main.queryByText(w.boostWaits)).toBeNull();
}

export const OthersSee: Story = {
  globals: { locale: "en" },
  parameters: laylaAsksToSam,
  play: async (c) => othersSee(c, "en"),
};

/** Before Sam boosts layla-asks, whose author approves Sam's boosts, the boost's menu says it will wait. */
async function boostSaysBefore(context: StoryContext, lang: Lang) {
  const { w, main, body } = await settled(context, lang);
  await userEvent.click(within(await postOf(main, "Translating a poem")).getByRole("button", { name: w.boost }));
  const menu = await body.findByRole("menu", { name: w.boostMenu });
  await atRest(context.canvasElement);
  await expect(menu).toHaveTextContent(w.approval);
}

export const BoostSaysBefore: Story = {
  globals: { locale: "en" },
  parameters: laylaAsksToSam,
  play: async (c) => boostSaysBefore(c, "en"),
};
export const BoostSaysBeforeJapanese: Story = {
  globals: { locale: "ja" },
  parameters: laylaAsksToSam,
  play: async (c) => boostSaysBefore(c, "ja"),
};

/** The reply composer of the screen's post whose text holds `text`, opened from its Reply. */
async function replyComposer(context: StoryContext, lang: Lang, text: string) {
  const { w, main, body } = await settled(context, lang);
  await userEvent.click(within(await postOf(main, text)).getByRole("button", { name: w.reply }));
  const dialog = within(await body.findByRole("dialog", { name: w.replyDialog }));
  await expect(await dialog.findByRole("textbox")).toBeVisible();
  return { w, body, dialog };
}

/** A reply to layla-asks, whose author approves the reader's replies: the composer says it waits, before it is sent. */
async function replyWaits(context: StoryContext, lang: Lang) {
  const { w, dialog } = await replyComposer(context, lang, "Translating a poem");
  await expect(await dialog.findByText(w.replyWaits)).toBeVisible();
  await atRest(context.canvasElement);
}

export const ReplyWaits: Story = {
  globals: { locale: "en" },
  parameters: laylaAsks,
  play: async (c) => replyWaits(c, "en"),
};
export const ReplyWaitsJapanese: Story = {
  globals: { locale: "ja" },
  parameters: laylaAsks,
  play: async (c) => replyWaits(c, "ja"),
};

/** A reply to a post that admits the reader at once: nothing says it waits. */
async function replyAtOnce(context: StoryContext, lang: Lang) {
  const { w, dialog } = await replyComposer(context, lang, "今朝の尾根");
  await expect(dialog.queryByText(w.approval)).toBeNull();
  await atRest(context.canvasElement);
}

export const ReplyAtOnce: Story = {
  globals: { locale: "en" },
  parameters: screen({ at: "yuki-ridge", viewer: "sam", server: { approvals: true } }),
  play: async (c) => replyAtOnce(c, "en"),
};

/**
 * In the composer's audience menu, who may reply, boost and quote: a row per kind with who may now, each opening its
 * audiences, and under any audience but Anyone whether others may ask for approval. Approval chosen for each kind shows
 * in its row, and the audience tool's name says the post is limited, before it is sent; Anyone again takes the check
 * away with its approval.
 */
async function choosesApproval(context: StoryContext, lang: Lang) {
  const { w, dialog, body } = await replyComposer(context, lang, "Translating a poem");
  await userEvent.click(dialog.getByRole("button", { name: w.audience }));
  const menu = () => within(body.getByRole("dialog", { name: w.audienceMenu }));
  await userEvent.click(await body.findByRole("button", { name: w.permissions }));
  const row = (kind: Kind) => menu().getByRole("button", { name: new RegExp(`^${w.kind[kind]} `) });
  await waitFor(() => expect(row("reply")).toBeVisible());
  for (const kind of ["reply", "announce", "quote"] as const)
    await expect(row(kind)).toHaveAccessibleName(`${w.kind[kind]} ${w.everyone}`);

  /** Opens the panel of `kind`, chooses `audience` and sets the check to `approval` (none: it must not show), back. */
  const choose = async (kind: Kind, audience: string, approval: boolean | undefined) => {
    await userEvent.click(row(kind));
    const audiences = within(await body.findByRole("radiogroup", { name: w.kind[kind] }));
    await userEvent.click(audiences.getByRole("radio", { name: audience }));
    await expect(audiences.getByRole("radio", { name: audience })).toHaveAttribute("aria-checked", "true");
    if (approval === undefined) await expect(menu().queryByRole("checkbox", { name: w.asks })).toBeNull();
    else {
      const check = menu().getByRole("checkbox", { name: w.asks });
      if ((check.getAttribute("aria-checked") === "true") !== approval) await userEvent.click(check);
      await expect(check).toHaveAttribute("aria-checked", String(approval));
    }
    await userEvent.click(menu().getByRole("button", { name: w.kind[kind] }));
    await waitFor(() => expect(row(kind)).toBeVisible());
  };

  // Anyone admits everyone at once: there is nothing to ask for.
  await choose("announce", w.everyone, undefined);
  await choose("reply", w.followers, true);
  await choose("announce", w.followers, true);
  await choose("quote", w.nobody, true);
  await expect(row("reply")).toHaveAccessibleName(`${w.kind.reply} ${w.withApproval(w.followers)}`);
  await expect(row("announce")).toHaveAccessibleName(`${w.kind.announce} ${w.withApproval(w.followers)}`);
  await expect(row("quote")).toHaveAccessibleName(`${w.kind.quote} ${w.withApproval(w.nobody)}`);
  await expect(dialog.getByRole("button", { name: w.limited })).toBeInTheDocument();

  // Anyone again: the check goes, and so does the approval it held; Only you without it admits no one else.
  await choose("announce", w.everyone, undefined);
  await expect(row("announce")).toHaveAccessibleName(`${w.kind.announce} ${w.everyone}`);
  await choose("quote", w.nobody, false);
  await expect(row("quote")).toHaveAccessibleName(`${w.kind.quote} ${w.nobody}`);
  await atRest(context.canvasElement);
}

export const ChoosesApproval: Story = {
  globals: { locale: "en" },
  parameters: laylaAsks,
  play: async (c) => choosesApproval(c, "en"),
};
export const ChoosesApprovalJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  parameters: laylaAsks,
  play: async (c) => choosesApproval(c, "ja"),
};

const waitingScreen = screen({ at: "/notifications/waiting", server: { approvals: true } });

/**
 * The screen of what waits: the reply, the boost and the quote, each with Decline and Approve. The reply approved and
 * the boost declined keep their answer in place of their buttons; back on the notifications, the row counts the quote
 * alone.
 */
async function answered(context: StoryContext, lang: Lang) {
  const { w, main } = await settled(context, lang);
  const feed = within(await main.findByRole("feed", { name: w.waiting }));
  const request = (asked: RegExp) => feed.getByRole("article", { name: asked });
  await waitFor(() => expect(feed.getAllByRole("article")).toHaveLength(3));
  for (const asked of [w.askedReply, w.askedBoost, w.askedQuote])
    await expect(
      within(request(asked))
        .getAllByRole("button")
        .map((b) => b.textContent),
    ).toEqual([w.decline, w.approve]);
  await userEvent.click(within(request(w.askedReply)).getByRole("button", { name: w.approve }));
  await waitFor(() => expect(within(request(w.askedReply)).queryAllByRole("button")).toHaveLength(0));
  await expect(request(w.askedReply)).toHaveTextContent(w.accepted);
  await userEvent.click(within(request(w.askedBoost)).getByRole("button", { name: w.decline }));
  await waitFor(() => expect(within(request(w.askedBoost)).queryAllByRole("button")).toHaveLength(0));
  await expect(request(w.askedBoost)).toHaveTextContent(w.declined);
  await expect(within(request(w.askedQuote)).getAllByRole("button")).toHaveLength(2);
  await userEvent.click(main.getByRole("button", { name: w.notifications }));
  await waitingRow(main, w, 1);
}

// Last in the file: an answer leaves an entry in React Aria's live announcer, which outlives the story.
export const Answered: Story = {
  globals: { locale: "en" },
  parameters: waitingScreen,
  play: async (c) => answered(c, "en"),
};
export const AnsweredJapanesePC: Story = {
  globals: { locale: "ja", ...pc },
  parameters: waitingScreen,
  play: async (c) => answered(c, "ja"),
};

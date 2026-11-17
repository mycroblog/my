// SPDX-License-Identifier: AGPL-3.0-only

// Asking for the author's approval to reply, boost and quote (ADR-0155, #3113), on the fake server's posts that approve
// by hand (`approvals`): the composer's choice of who may reply, boost and quote, with others asking for approval, and
// what says that a reply or a boost will wait; what waits for the reader's approval, counted over the notifications
// and approved or declined on its screen; and the lines over the reader's reply and boost that wait or were not
// approved, which no one else sees.
//
// The fake's reader is yuki: marco asks to reply to and boost yuki-asks and mio to quote it; layla-asks admits replies
// and boosts of the people it mentions at once and of everyone else by approval, and holds yuki's reply that waits,
// yuki's reply that was not approved, and yuki's boost that waits. A play presses what a press alone reaches (a menu,
// the composer, an answer in a request's place), in the story's locale, ends once the page is at rest, and asserts
// nothing: the features' tests hold what each state shows.

import { featureStringsIn } from "@my/app-features";
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { userEvent, waitFor, within } from "storybook/test";
import { screen, screenMeta } from "./app.samples";

const meta = {
  title: "Approval states",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/notifications", server: { approvals: true } }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen in the canvas once it shows its state: its main region, the page's body (for popovers), the words. */
async function settled(context: StoryContext) {
  await screenMeta.play(context);
  const root = context.canvasElement;
  const { t } = featureStringsIn(String(context.globals["locale"] ?? "en"));
  return { t, main: within(within(root).getByRole("main")), body: within(root.ownerDocument.body) };
}

type Queries = Awaited<ReturnType<typeof settled>>["main"];

/** Waits until nothing on the page moves: the composer's entrance, a menu's, a panel's slide. */
const atRest = (root: HTMLElement) =>
  waitFor(() => {
    if (root.ownerDocument.getAnimations().some((a) => a.playState === "running"))
      throw new Error("the page is still moving");
  });

/** The article of the screen's post whose text holds `text`. */
async function postOf(main: Queries, text: string): Promise<HTMLElement> {
  const found = (await main.findAllByRole("article")).find((a) => a.textContent.includes(text));
  if (found === undefined) throw new Error(`no post holds ${text}`);
  return found;
}

/**
 * The Reply and the Boost of `post`, in any locale: Boost is the first of its buttons that opens a menu, and Reply the
 * button before it.
 */
function actionsOf(post: HTMLElement) {
  const buttons = within(post).getAllByRole("button");
  const boost = buttons.findIndex((b) => b.getAttribute("aria-haspopup") === "true");
  const [reply, boosts] = [buttons[boost - 1], buttons[boost]];
  if (reply === undefined || boosts === undefined) throw new Error("the post has no actions");
  return { reply, boost: boosts };
}

/** Over the notifications, a row counts the three requests and opens their screen. */
export const Counted: Story = {};
export const CountedJapanesePC: Story = {};

const laylaAsks = screen({ at: "layla-asks", server: { approvals: true } });
const laylaAsksToSam = screen({ at: "layla-asks", viewer: "sam", server: { approvals: true } });

/** Under layla-asks, the reader's waiting boost and replies each carry their line. */
export const OwnLines: Story = { parameters: laylaAsks };
export const OwnLinesJapaneseMinimumWidth: Story = { parameters: laylaAsks };

/** Someone else reads layla-asks: neither yuki's reply that waits nor the one declined, and no line. */
export const OthersSee: Story = { parameters: laylaAsksToSam };

/** Before Sam boosts layla-asks, whose author approves Sam's boosts, the boost's menu says it will wait. */
async function boostSaysBefore(context: StoryContext) {
  const { main, body } = await settled(context);
  await userEvent.click(actionsOf(await postOf(main, "Translating a poem")).boost);
  await body.findByRole("menu");
  await atRest(context.canvasElement);
}

export const BoostSaysBefore: Story = { parameters: laylaAsksToSam, play: boostSaysBefore };
export const BoostSaysBeforeJapanese: Story = { parameters: laylaAsksToSam, play: boostSaysBefore };

/** The reply composer of the screen's post whose text holds `text`, opened from its Reply. */
async function replyComposer(context: StoryContext, text: string) {
  const { t, main, body } = await settled(context);
  await userEvent.click(actionsOf(await postOf(main, text)).reply);
  const dialog = await waitFor(() => {
    const found = body.getAllByRole("dialog").find((d) => within(d).queryAllByRole("textbox").length > 0);
    if (found === undefined) throw new Error("the composer has not come yet");
    return found;
  });
  return { t, body, dialog };
}

/** A reply to `text`, with the composer left open once it is at rest. */
const replying = (text: string) => async (context: StoryContext) => {
  await replyComposer(context, text);
  await atRest(context.canvasElement);
};

/** A reply to layla-asks, whose author approves the reader's replies: the composer says it waits, before it is sent. */
export const ReplyWaits: Story = { parameters: laylaAsks, play: replying("Translating a poem") };
export const ReplyWaitsJapanese: Story = { parameters: laylaAsks, play: replying("Translating a poem") };

/** A reply to a post that admits the reader at once: nothing says it waits. */
export const ReplyAtOnce: Story = {
  parameters: screen({ at: "yuki-ridge", viewer: "sam", server: { approvals: true } }),
  play: replying("今朝の尾根"),
};

type Kind = "reply" | "announce" | "quote";
type Audience = "everyone" | "followers" | "nobody";

/**
 * In the composer's audience menu, who may reply, boost and quote: a submenu with a row per kind with who may now, each
 * opening its audiences, and under any audience but Anyone whether others may ask for approval. It leaves the submenu
 * open with followers replying and boosting, the author alone quoting, and others asking for approval.
 */
async function choosesApproval(context: StoryContext) {
  const { t, dialog, body } = await replyComposer(context, "Translating a poem");
  // The audience's tool is the second of the composer's buttons that open a menu, after the drafts' in its head.
  const tool = within(dialog)
    .getAllByRole("button")
    .filter((b) => b.getAttribute("aria-haspopup") === "true")[1];
  if (tool === undefined) throw new Error("the composer has no audience");
  await userEvent.click(tool);
  const starts = (label: string) => (name: string) => name.startsWith(label);
  await userEvent.click(await body.findByRole("menuitem", { name: starts(t("compose.permissions")) }));
  const row = (kind: Kind) => body.findByRole("menuitem", { name: starts(t(`compose.interaction.${kind}`)) });

  /** Opens the submenu of `kind`, chooses `audience` with others asking for approval, and goes back. */
  const choose = async (kind: Kind, audience: Audience) => {
    await userEvent.click(await row(kind));
    const menu = await body.findByRole("menu", { name: t(`compose.interaction.${kind}`) });
    await userEvent.click(within(menu).getByRole("menuitemradio", { name: t(`compose.audience.${audience}`) }));
    await userEvent.click(await within(menu).findByRole("menuitemcheckbox", { name: t("compose.approval") }));
    await within(menu).findByRole("menuitemcheckbox", { checked: true });
    // Back heads the submenu, beside its items: the one button there that Tab reaches.
    const back = within(menu.parentElement ?? menu)
      .getAllByRole("button")
      .find((b) => b.tabIndex !== -1);
    if (back === undefined) throw new Error("the submenu has no head");
    await userEvent.click(back);
    await row(kind);
  };

  await choose("reply", "followers");
  await choose("announce", "followers");
  await choose("quote", "nobody");
  await atRest(context.canvasElement);
}

export const ChoosesApproval: Story = { parameters: laylaAsks, play: choosesApproval };
export const ChoosesApprovalJapanesePC: Story = { parameters: laylaAsks, play: choosesApproval };

const waitingScreen = screen({ at: "/notifications/waiting", server: { approvals: true } });

/**
 * The screen of what waits, the newest first: the boost, the quote and the reply, each with Decline and Approve. The
 * reply approved and the boost declined keep their answer in place of their buttons.
 */
async function answered(context: StoryContext) {
  const { main } = await settled(context);
  const feed = within(await main.findByRole("feed"));
  const requests = await waitFor(() => {
    const found = feed.getAllByRole("article");
    if (found.length < 3) throw new Error("the requests have not all come yet");
    return found;
  });
  const [boost, reply] = [requests[0], requests.at(-1)];
  // Decline is the first of a request's pair in any locale, Approve the second.
  const [approve, decline] = [
    reply && within(reply).getAllByRole("button")[1],
    boost && within(boost).getAllByRole("button")[0],
  ];
  if (boost === undefined || reply === undefined || approve === undefined || decline === undefined)
    throw new Error("the requests have no buttons");
  await userEvent.click(approve);
  await waitFor(() => {
    if (within(reply).queryAllByRole("button").length > 0) throw new Error("the reply is not answered yet");
  });
  await userEvent.click(decline);
  await waitFor(() => {
    if (within(boost).queryAllByRole("button").length > 0) throw new Error("the boost is not answered yet");
  });
}

// Last in the file: an answer leaves an entry in React Aria's live announcer, which outlives the story.
export const Answered: Story = { parameters: waitingScreen, play: answered };
export const AnsweredJapanesePC: Story = { parameters: waitingScreen, play: answered };

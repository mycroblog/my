// SPDX-License-Identifier: AGPL-3.0-only

// Reporting to the moderators (FR-701, FR-709), in the reader's language (the toolbar's locale), over the page as the
// app opens it: from a post's menu (that post chosen, the account's other posts to add), from a profile's menu (none
// chosen), from a notification's (a local account, so nothing to forward), from a voice chat room's and an event's
// menu (no posts, never forwarded); a violation with the rules it breaks, a remote account's report sent to its server
// too, the account's posts loading, failing and none, more posts on their way, failing, and read; sending, refused by the server, and a
// comment it refused; and once sent, with mute and block, one muted and one that failed, and for an account the reader
// mutes already. Each answers the reader's choices as the feature does, and sends nothing.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { accountOf, posts, statusOf } from "./post.samples";
import {
  reportDraft,
  ReportPanel,
  type ReportAfter,
  type ReportDraft,
  type ReportPanelProps,
  type ReportPost,
  type ReportPosts,
  type ReportRule,
  type ReportTarget,
} from "./report";

const byId = (id: string) => {
  const post = posts.find((p) => p.id === id);
  if (post === undefined) throw new Error(`no sample post ${id}`);
  return statusOf(post, id);
};

/** The reported account's posts, newest first: one behind a content warning among them. */
const accountPosts: ReportPost[] = ["lena-quiet", "takuya-cw", "ravi-two", "mika-long", "hana-four"].map(byId);
/** Older posts of the account, as Show more adds them. */
const olderPosts: ReportPost[] = ["ravi-two", "mika-long", "hana-four"].map((id) => {
  const post = posts.find((p) => p.id === id);
  if (post === undefined) throw new Error(`no sample post ${id}`);
  return statusOf(post, `${id}-older`);
});
const [first] = accountPosts;
if (first === undefined) throw new Error("no sample posts");

const remote = accountOf({ name: "Lena Novak", handle: "@lena@toot.example.eu" });
const local = accountOf({ name: "Kenji Watanabe", handle: "@kenji@my.example" });

const rules: ReportRule[] = [
  { id: "r1", text: "Be kind to each other.", hint: "No insults, threats or pile-ons." },
  { id: "r2", text: "Credit the photographer when you share a photo." },
  { id: "r3", text: "Mark sensitive media as sensitive." },
  { id: "r4", text: "No advertising without the moderators' permission." },
];

const read = (over: Partial<ReportPosts> = {}): ReportPosts => ({
  items: accountPosts,
  status: "success",
  more: true,
  loadingMore: false,
  failedMore: false,
  onMore: () => undefined,
  onRetry: () => undefined,
  ...over,
});

const ready: ReportAfter = { state: "ready", onPress: () => undefined };

type Shown = Omit<ReportPanelProps, "onChange" | "onSend" | "onClose" | "draft" | "instance" | "rules"> & {
  draft?: ReportDraft | undefined;
  rules?: readonly ReportRule[] | undefined;
};

/** The panel over the page, holding what the reader chooses. */
function Panel({ draft: start, rules: shownRules = rules, ...props }: Shown) {
  const [draft, setDraft] = useState(start ?? reportDraft());
  return (
    <ReportPanel
      {...props}
      instance="mycroblog demo"
      rules={shownRules}
      draft={draft}
      onChange={setDraft}
      onSend={() => undefined}
      onClose={() => undefined}
    />
  );
}

const fromPost: ReportTarget = { kind: "account", account: remote };

const meta = {
  title: "Report",
  tags: ["themes"],
  component: Panel,
  parameters: { layout: "fullscreen" },
} satisfies Meta<typeof Panel>;
export default meta;

type Story = StoryObj<typeof meta>;

/** From a post's menu: the post chosen, the account's other posts to add, and older ones on request. */
export const FromPost: Story = {
  args: { target: fromPost, posts: read(), draft: reportDraft(first.id), phase: "writing" },
};
/** From a profile's menu: no post chosen yet. */
export const FromProfile: Story = { args: { target: fromPost, posts: read(), phase: "writing" } };
/** From a notification's menu, of an account of this server: its post chosen, and nothing to forward. */
export const FromNotification: Story = {
  args: {
    target: { kind: "account", account: local },
    posts: read({ more: false }),
    draft: reportDraft(first.id),
    phase: "writing",
  },
};
/** From a voice chat room's menu: the room and its host, with no posts, never forwarded. */
export const FromRoom: Story = {
  args: { target: { kind: "room", account: remote, name: "Late-night radio" }, phase: "writing" },
};
/** From an event's menu: the event and its organizer, with no posts, never forwarded. */
export const FromEvent: Story = {
  args: { target: { kind: "event", account: local, name: "Book swap at the library" }, phase: "writing" },
};
/** A break of the server's rules: the rules it breaks, two of them chosen. */
export const Violation: Story = {
  args: {
    target: fromPost,
    posts: read(),
    draft: { ...reportDraft(first.id), category: "violation", rules: ["r1", "r4"] },
    phase: "writing",
  },
};
/** A server without rules offers no break of them. */
export const NoRules: Story = { args: { target: fromPost, posts: read(), rules: [], phase: "writing" } };
/** A remote account's report, sent to the moderators of its server too, with a comment. */
export const RemoteForward: Story = {
  args: {
    target: fromPost,
    posts: read(),
    draft: {
      ...reportDraft(first.id),
      category: "spam",
      comment: "The same link in every reply to my posts this week.",
      forward: true,
    },
    phase: "writing",
  },
};
/** The account's posts loading. */
export const PostsLoading: Story = {
  args: { target: fromPost, posts: read({ items: [], status: "pending", more: false }), phase: "writing" },
};
/** The account's posts could not be read: Try again. */
export const PostsFailed: Story = {
  args: { target: fromPost, posts: read({ items: [], status: "error", more: false }), phase: "writing" },
};
/** The reader sees none of the account's posts. */
export const PostsNone: Story = {
  args: { target: fromPost, posts: read({ items: [], more: false }), phase: "writing" },
};
/** Older posts on their way. */
export const PostsMore: Story = {
  args: { target: fromPost, posts: read({ loadingMore: true }), draft: reportDraft(first.id), phase: "writing" },
};
/** After Show more: the older posts under those read first, and no Show more once the account has no more. */
export const PostsOlder: Story = {
  args: {
    target: fromPost,
    posts: read({ items: [...accountPosts, ...olderPosts], more: false }),
    draft: reportDraft(first.id),
    phase: "writing",
  },
};
/** Older posts could not be read: those read stay, Show more named Try again under the sentence that says so. */
export const PostsMoreFailed: Story = {
  args: {
    target: fromPost,
    posts: read({ status: "error", failedMore: true }),
    draft: reportDraft(first.id),
    phase: "writing",
  },
};
/** The report on its way: Send spins and the choices hold. */
export const Sending: Story = {
  args: {
    target: fromPost,
    posts: read(),
    draft: { ...reportDraft(first.id), category: "spam", forward: true },
    phase: "sending",
  },
};
/** Refused by the server, in its words, right before Send; the choices stay as they were. */
export const Refused: Story = {
  args: {
    target: fromPost,
    posts: read(),
    draft: { ...reportDraft(first.id), category: "spam" },
    phase: "writing",
    refusal: "You have sent too many reports. Wait an hour, then try again.",
  },
};
/** A comment the server refused: its error under it, and the focus in it. */
export const CommentRefused: Story = {
  args: {
    target: fromPost,
    posts: read(),
    draft: { ...reportDraft(first.id), category: "other", comment: "…" },
    phase: "writing",
    refusal: "The comment is too long.",
    commentRefused: true,
  },
};
/** Sent: what comes of it, then mute and block. */
export const Sent: Story = { args: { target: fromPost, phase: "sent", mute: ready, block: ready } };
/** Sent, the account muted from it, and a block that failed. */
export const SentMutedBlockFailed: Story = {
  args: {
    target: fromPost,
    phase: "sent",
    mute: { state: "done", onPress: () => undefined },
    block: { state: "failed", onPress: () => undefined },
  },
};
/** Sent, of an account the reader mutes already: block alone. */
export const SentMuting: Story = { args: { target: fromPost, phase: "sent", block: ready } };

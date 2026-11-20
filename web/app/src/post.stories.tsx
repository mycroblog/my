// SPDX-License-Identifier: AGPL-3.0-only

// A post's page, a story per state: a reply in its thread, a post alone with no replies, a post with reactions and who
// reacted under it, a thread folded (both folds), its posts in another language offered for translation by a provider
// that answers, and by one that fails, and each state every screen is shown in (app.samples.tsx); an article's page;
// and the servers on which a translation goes each of its ways; a visitor's thread, and a followers-only post a visitor
// may not see, which is not found.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta } from "./app.samples";

const thread = "yuki-ridge-reply";

const meta = {
  title: "Post",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: thread }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const Thread: Story = {};
export const Alone: Story = { parameters: screen({ at: "mio-hello" }) };
/** A post with reactions: a row per emoji under it, with the faces of who reacted, each leading to its list. */
export const WhoReacted: Story = { parameters: screen({ at: "yuki-ridge" }) };
/** With both folds: the earlier posts and the replies the page does not show at first. */
export const Folded: Story = { parameters: screen({ at: thread, state: "folded" }) };
export const Empty: Story = { parameters: screen({ at: thread, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at: thread, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at: thread, state: "error" }) };
export const LongText: Story = { parameters: screen({ at: thread, state: "long" }) };
/** Read without a session: the actions that need an account lead to the sign-in, coming back to the post. */
export const Visitor: Story = { parameters: screen({ at: thread, signedOut: true }) };
/** A followers-only post opened by a visitor: the server answers that it is not there. */
export const VisitorNotFound: Story = { parameters: screen({ at: "ren-followers", signedOut: true }) };
/**
 * With a translation provider on the server: the posts in another language than the reader's offer Translate (in en,
 * the Japanese ones; in ja, the English ones), which the provider answers.
 */
export const Translatable: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "answers" } } }),
};
/** With a provider on the server that fails: Translate tells it, with Try again. */
export const TranslationFailed: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "failed" } } }),
};
/**
 * A post whose author approves replies and boosts by hand (ADR-0155): the reader's boost of it says it waits, and the
 * reader's replies under it, which the reader alone sees, say one waits and the other was not approved.
 */
export const WaitingForApproval: Story = { parameters: screen({ at: "layla-asks", server: { approvals: true } }) };
/** An article's page (ADR-0160): its head, its text in full for long reading, its actions, and its replies under it. */
export const Article: Story = { parameters: screen({ at: "sam-article" }) };

// The servers on which a translation goes another way once the reader presses Translate, beside the provider that
// answers and the one that fails above. What the post then shows is in the Post stories of @my/app-components, by
// props.

/** No provider may receive the post: the browser's translator, where the browser has one, translates it. */
export const TranslateInBrowser: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "unavailable", browser: true } } }),
};

/**
 * Over the server's limit: Translate offers the browser's translator where the browser has one, and else says when
 * translation comes back.
 */
export const TranslationLimit: Story = {
  parameters: screen({ at: thread, server: { translation: { provider: "limit", browser: true } } }),
};

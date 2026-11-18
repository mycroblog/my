// SPDX-License-Identifier: AGPL-3.0-only

// Who may reply to, boost and quote a post, as the composer offers it for one post (ADR-0155) and the settings for
// the account's new posts: the kinds, the audiences one may admit at once, whether everyone else may ask for the
// author's approval, and how a choice reads. One list for both, so that the settings offer what the composer can
// choose and nothing else.

import type { Audience, Interaction, InteractionChoice } from "@my/app-data";
import type { stringsIn } from "./locale";

type T = ReturnType<typeof stringsIn>["t"];

/** The kinds of interaction with a post that its author decides about, in the order they are listed. */
export const interactions: readonly Interaction[] = ["reply", "announce", "quote"];

/** The audiences one may admit at once, from the widest. */
export const audiences: readonly Audience[] = ["everyone", "followers", "mentioned", "nobody"];

/** Whether a choice lets others ask for approval: asking means nothing while anyone may. */
export const asksApproval = (choice: InteractionChoice): boolean => choice.approval && choice.audience !== "everyone";

/**
 * Every choice there is for one kind: each audience at once, and, for an audience other than anyone, the same with
 * everyone else by the author's approval.
 */
export const interactionChoices: readonly InteractionChoice[] = audiences.flatMap((audience): InteractionChoice[] =>
  audience === "everyone"
    ? [{ audience, approval: false }]
    : [
        { audience, approval: false },
        { audience, approval: true },
      ],
);

/** Who may do it, as a value: the audience, and that others may ask, which anyone needs not. */
export const choiceText = (choice: InteractionChoice, t: T): string =>
  t("compose.permission.now", {
    audience: t(`compose.audience.${choice.audience}`),
    approval: asksApproval(choice) ? "yes" : "no",
  });

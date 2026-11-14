// SPDX-License-Identifier: AGPL-3.0-only
// What a screen passes as each post's `onAction`, for the tests that press a post's action: a test renders without a
// DOM, so it cannot press a button, and calls the handler the screen gave the post instead. Post and Column of
// @my/app-components are called through, in the same place of the tree, so that what they draw is as it was.

import { mock } from "bun:test";
import * as components from "@my/app-components";
import type { ColumnProps, PostAction, PostProps } from "@my/app-components";

type OnAction = (action: PostAction) => void;

/** The `onAction` each post was last drawn with, by the ID of the status it shows. */
const handlers = new Map<string, OnAction>();

const RealPost = components.Post;
const RealColumn = components.Column;

await mock.module("@my/app-components", () => ({
  ...components,
  Post: (props: PostProps) => {
    if (props.onAction !== undefined) handlers.set((props.status.boost ?? props.status).id, props.onAction);
    return RealPost(props);
  },
  Column: (props: ColumnProps) => {
    const { onAction } = props;
    if (onAction !== undefined) for (const s of props.statuses) handlers.set((s.boost ?? s).id, onAction);
    return RealColumn(props);
  },
}));

/** Presses the action on the post of the status with the ID given, as the screen last drew it. */
export function press(action: PostAction): void {
  const handler = handlers.get(action.id);
  if (handler === undefined) throw new Error(`test: no post of ${action.id} was drawn with an onAction`);
  handler(action);
}

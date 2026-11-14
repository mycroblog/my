// SPDX-License-Identifier: AGPL-3.0-only

// The presentational components shared across the features of the web client (ADR-0025 §2, §4): built on
// @my/design-system, they take their data as props and fetch nothing. They write no Tailwind class of their own.

export { AccountLine, AccountName, handleOf, nameOf } from "./account";
export {
  Column,
  FeedKeys,
  ListColumn,
  moveFocus,
  type ColumnProps,
  type ColumnTimeline,
  type ListColumnProps,
  type ListWords,
} from "./column";
export { accountPath, statusPath } from "./paths";
export { Post, type PostAction, type PostProps } from "./post";
export { PostContent, type ContentEmoji, type PostContentProps } from "./post-content";
export { isRemote, QuoteContext, ReplyContext } from "./post-context";
export { PostDetails } from "./post-details";
export type {
  PostAccount,
  PostMedia,
  PostMention,
  PostReaction,
  PostStatus,
  PostTranslation,
  PostVisibility,
} from "./status";

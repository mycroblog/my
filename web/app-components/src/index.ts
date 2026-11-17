// SPDX-License-Identifier: AGPL-3.0-only

// The presentational components shared across the features of the web client (ADR-0025 §2, §4): built on
// @my/design-system, they take their data as props and fetch nothing. They write no Tailwind class of their own.

export { AccountFollow, AccountLine, AccountName, handleOf, nameOf, type FollowState } from "./account";
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
export { translateContent, translatePlain, type TranslateText } from "./content-translation";
export { DeleteConfirm, type DeleteConfirmProps } from "./delete-confirm";
export { EditHistory, type EditHistoryProps, type PostEdit } from "./edit-history";
export { languageName } from "./locale";
export {
  accountPath,
  followRequestsPath,
  hashtagPath,
  heldOfPath,
  heldPath,
  notificationPath,
  reactorsPath,
  statusPath,
  tagPath,
  waitingPath,
  type NotificationTarget,
} from "./paths";
export { PollCard, PollOption, PollOptions } from "./compose";
export { Post, type PostAction, type PostPicker, type PostProps, type TranslationFailure } from "./post";
export { excerptHtml, PostContent, ServerDomain, type ContentEmoji, type PostContentProps } from "./post-content";
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
export { ReactorsTitle, WhoReacted, type Reactors } from "./who-reacted";

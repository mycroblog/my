// SPDX-License-Identifier: AGPL-3.0-only

// The presentational components shared across the features of the web client (ADR-0025 §2, §4): built on
// @my/design-system, they take their data as props and fetch nothing. They write no Tailwind class of their own.

export {
  AccountFollow,
  AccountLine,
  AccountName,
  handleOf,
  nameOf,
  sentenceHandleOf,
  type FollowState,
} from "./account";
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
export {
  ConversationAuthor,
  ConversationFaces,
  ConversationHead,
  ConversationLine,
  ConversationReply,
  ConversationText,
  type ConversationLineProps,
} from "./conversation";
export { translateContent, translatePlain, type TranslateText } from "./content-translation";
export { DeleteConfirm, type DeleteConfirmProps } from "./delete-confirm";
export {
  DraftCount,
  DraftList,
  DraftRow,
  DraftsEmpty,
  DraftSection,
  DraftSections,
  localInput,
  localInstant,
  readerZone,
  ScheduleForm,
  ScheduleTime,
  scheduleTime,
  ScheduleZone,
  type DraftMark,
  type DraftRowProps,
  type ScheduleFormProps,
} from "./drafts";
export { plainText } from "./diff";
export { EditHistory, type EditHistoryProps, type PostEdit } from "./edit-history";
export { EventListLine, useGoingCount, type EventShown } from "./event-line";
export {
  eventMoment,
  eventWhen,
  useEventMoment,
  useEventTime,
  useEventWhen,
  useMomentLine,
  zonePlace,
  type EventMoment,
  type EventSpan,
  type EventTimed,
  type EventWhen,
} from "./event-time";
export { languageName } from "./locale";
export {
  accountMovePath,
  accountPath,
  autoDeletePath,
  dataExportPath,
  eventCoorganizersPath,
  eventEditPath,
  eventInvitationsPath,
  eventNewPath,
  eventPath,
  eventRemindersPath,
  eventsPath,
  followRequestsPath,
  hashtagPath,
  heldOfPath,
  heldPath,
  notificationPath,
  notificationSettingsPath,
  reactorsPath,
  costsPagePath,
  statusPath,
  supporterSettingsPath,
  supportersPagePath,
  supporterTipPath,
  tagPath,
  voiceHistoryPath,
  voiceInvitationsPath,
  voiceOpenPath,
  voiceRoomPath,
  voiceRoomSettingsPath,
  voiceRoomsPath,
  waitingPath,
  type NotificationTarget,
} from "./paths";
export { PollCard, PollOption, PollOptions } from "./compose";
export {
  FieldList,
  MetaCount,
  MetaLine,
  NoteEditor,
  NoteLine,
  ProfileHead,
  TagList,
  type ProfileHeadProps,
} from "./profile";
export { Post, type PostAction, type PostPicker, type PostProps, type TranslationFailure } from "./post";
export { excerptHtml, PostContent, ServerDomain, type ContentEmoji, type PostContentProps } from "./post-content";
export { isRemote, QuoteContext, ReplyContext } from "./post-context";
export { PostDetails } from "./post-details";
export { FilterSentence } from "./post-filter";
export {
  reportDraft,
  ReportPanel,
  reportPanelLimits,
  type ReportAccount,
  type ReportAfter,
  type ReportCategory,
  type ReportDraft,
  type ReportPanelProps,
  type ReportPost,
  type ReportPosts,
  type ReportRule,
  type ReportTarget,
} from "./report";
export type {
  PostAccount,
  PostFiltered,
  PostMedia,
  PostMention,
  PostReaction,
  PostStatus,
  PostTranslation,
  PostVisibility,
} from "./status";
export {
  RoomCaption,
  roomIcon,
  useHeadcount,
  VoiceRoomEntryLine,
  VoiceRoomLine,
  type RoomEntryShown,
  type RoomIcon,
  type RoomShown,
} from "./voice-room";
export { ReactorsTitle, WhoReacted, type Reactors } from "./who-reacted";

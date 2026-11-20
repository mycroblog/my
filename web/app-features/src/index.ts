// SPDX-License-Identifier: AGPL-3.0-only

// The features of the web client (ADR-0025 §2–§4), a directory each: what a screen of the app places, built from
// @my/app-components and fed by @my/app-data. A feature imports no other feature; the app wires them together.

export { parseAuthPage, type AuthPage, type ScreenName } from "./auth/data";
export {
  AnnouncementList,
  AnnouncementPage,
  loadAnnouncement,
  loadAnnouncements,
  useAnnouncementTitle,
} from "./announcements/announcements";
export { AuthScreen } from "./auth/screen";
export { descriptionParts } from "./events/description";
export { EventCreateScreen, EventEditScreen } from "./events/editor";
export { EventScreen, EventVisitor, loadEvent, useEventTitle } from "./events/event";
export { EventCoorganizersScreen, EventInvitationsScreen } from "./events/invitations";
export { EventsScreen, loadEvents } from "./events/list";
export type { JoinRoom } from "./events/places";
export { FundingMeterView } from "./finance/meter";
export { PublicFinanceScreen } from "./finance/public";
export { FinanceScreen } from "./finance/viewer";
export {
  CallUnavailableError,
  currentCall,
  inCall,
  joinCall,
  lastCallEnd,
  leaveCallBeforeSignOut,
  openCall,
  runCallsOn,
  useLastCallEnd,
  useVoiceCall,
  useVoiceCallState,
  type CallRuntime,
} from "./call/calls";
export { CallEnds } from "./call/ends";
export { cohostLimit, hostOperations, hostsOthers, mayActOn, opKey } from "./call/host";
export { VoiceInvitations } from "./call/invitations";
export { askLeave, clearLeaveAsk, leaveNeedsChoice, useLeaveAsk } from "./call/leave-ask";
export { loadVoiceRoom, useVoiceRoomTitle, VoiceRoomScreen, VoiceRoomVisitor } from "./call/room";
export { dismissRoomEnd, joinRoom, openRoom, useRoomEnd, type RoomEnd } from "./call/room-end";
export { VoiceRoomOpen, VoiceRoomSettingsScreen } from "./call/room-form";
export { loadVoiceRooms, VoiceRooms } from "./call/rooms";
export {
  endCall,
  enterCall,
  leaveCall,
  muteCall,
  resumeCallPlayback,
  updateCall,
  useCall,
  type Call,
  type CallControls,
} from "./call/store";
export {
  grantRenewalLead,
  heartbeatInterval,
  heartbeatTtl,
  reactionDuration,
  roomGrant,
  systemClock,
  VoiceCall,
  type CallEnd,
  type CallPhase,
  type Clock,
  type MicFailure,
  type ShownReaction,
  type VoiceCallRuntime,
  type VoiceCallState,
} from "./call/voice-call";
export { ClosedScreen, type ClosedScreenProps } from "./closure/closed";
export {
  ArticleEditor,
  ArticleView,
  type ArticleEditorProps,
  type ArticleLeaving,
  type ArticleOpening,
  type ArticleSpecimen,
  type ArticleViewProps,
} from "./compose/article";
export { emptyArticle, type ArticleDraft } from "./compose/article-model";
export {
  conversationPath,
  ConversationList,
  ConversationView,
  loadConversation,
  loadConversations,
  type ConversationReplyTo,
} from "./conversations/conversations";
export { DraftsView, type DraftsState, type DraftsViewProps } from "./compose/drafts";
export { InPlaceReply, type InPlaceReplyProps } from "./compose/in-place";
export { ComposeLayer, type ComposeLayerProps, type ComposeSpecimen } from "./compose/layer";
export {
  emptyDraft as emptyComposeDraft,
  type Draft as ComposeDraft,
  type Opening as ComposeOpening,
  type Target as ComposeTarget,
} from "./compose/model";
export { LatestNotifications } from "./notifications/latest";
export { loadNotifications, Notifications } from "./notifications/notifications";
export { Held, HeldOf, loadHeld, loadHeldOf } from "./notifications/held";
export { loadWaitingRequests, WaitingRequests } from "./notifications/waiting";
export { FollowRequests, loadFollowRequests } from "./notifications/follow-requests";
export { PostActions, type ComposeRequest } from "./post-actions";
export { inAnotherLanguage, TranslatablePost, usePostTranslation, type PostTranslationState } from "./post-translation";
export { ReportDialog, reportRequest, type ReportAct, type ReportSubject } from "./report";
export { Follows, loadFollows } from "./profile/follows";
export { loadOwnProfile, loadProfile, OwnProfile, Profile, useProfileTitle } from "./profile/profile";
export { loadOwnApp, OwnApp, useOwnAppTitle } from "./settings/apps/app";
export { loadAppsSettings } from "./settings/apps/apps";
export { AuthorizedAppScreen, useAuthorizedAppTitle } from "./settings/apps/authorized";
export { appsPath, appsPaths, useAppsTitle, type AppScreenProps } from "./settings/apps/parts";
export { AppRegister, loadAppRegister } from "./settings/apps/register";
export { AppToken } from "./settings/apps/token";
export {
  confirmationKeptKey,
  forgetConfirmationKept,
  useConfirmation,
  useProviderReturn,
  type ConfirmationAct,
  type ConfirmationOptions,
  type Confirmed,
} from "./settings/confirmation";
export {
  ProfileAvatarField,
  profileEditPath,
  ProfileNameField,
  usePickedPicture,
  useProfileIdentity,
  type PickedPicture,
  type ProfileIdentity,
} from "./profile-identity";
export {
  loadSettingsEntry,
  loadSettingsFacts,
  settingsEntries,
  type SettingsAddress,
  type SettingsEntry,
  type SettingsEntryProps,
  type SettingsFacts,
  type SettingsGroupId,
  type SettingsSessionProps,
  type SettingsSubProps,
} from "./settings/entries";
export { InviteCreate } from "./settings/invites/create";
export { OwnInviteScreen, useOwnInviteTitle, type InviteScreenProps } from "./settings/invites/invite";
export { loadInvitesSettings } from "./settings/invites/invites";
export { invitesPath, invitesPaths, useInviteCreateTitle } from "./settings/invites/parts";
export { securityPath, securityPaths, useSecurityTitle } from "./settings/security/parts";
export { SecurityPasskey } from "./settings/security/passkey";
export { SecurityPassword } from "./settings/security/password";
export { loadSecuritySettings, SecuritySettings } from "./settings/security/security";
export { SecurityTwoFactor } from "./settings/security/two-factor";
export { loadSupporterTip, SupporterTip, useSupporterTipTitle } from "./settings/supporter-tip";
export { loadSupporters, SupportersScreen } from "./supporters/page";
export {
  loadModerationAction,
  ModerationActionScreen,
  useModerationActionTitle,
  type ModerationActionProps,
} from "./settings/account-action";
export { AccountStatusScreen, loadAccountStatus, useAccountStatusTitle } from "./settings/account-status";
export { AccountExportScreen, loadAccountExport, useAccountExportTitle } from "./settings/account-export";
export { AccountImportScreen, loadAccountImport, useAccountImportTitle } from "./settings/account-import";
export {
  AccountAutoDelete,
  loadAccountAutoDelete,
  useAccountAutoDeleteTitle,
  type AccountAutoDeleteProps,
} from "./settings/account-auto-delete";
export { AccountAliasesScreen, loadAccountAliases, useAccountAliasesTitle } from "./settings/account-aliases";
export { AccountMoveScreen, loadAccountMoveSettings, useAccountMoveTitle } from "./settings/account-move";
export {
  AccountDeactivateScreen,
  AccountDeleteScreen,
  loadAccountDeactivateSettings,
  loadAccountDeleteSettings,
  useAccountDeactivateTitle,
  useAccountDeleteTitle,
} from "./settings/account-end";
export { loadProfileEdit, ProfileEdit, useProfileEditTitle, type ProfileEditProps } from "./settings/profile-edit";
export { FilterEdit, loadFilterEdit, useFilterEditTitle, type FilterEditProps } from "./settings/filter-edit";
export { loadRelationshipSettings, RelationshipListScreen, useRelationshipListTitle } from "./settings/relationships";
export { SettingsBeside, settingsEntryPath, settingsFactsWaitMs, SettingsIndex } from "./settings/menu";
export {
  SettingsEntryScreen,
  SettingsMissing,
  useSettingsTitle,
  type SettingsEntryScreenProps,
} from "./settings/screen";
export { SignOutDialog, type SignOutDialogProps } from "./settings/sign-out";
export { UnsavedSettings } from "./settings/unsaved";
export { loadReactors, ReactorList } from "./thread/reactors";
export { loadThread, Thread } from "./thread/thread";
export { HomeTimeline, loadHomeTimeline } from "./timeline/home";
export { loadPublicTimeline, PublicTimeline } from "./timeline/public";
export { Bookmarks, loadBookmarks } from "./timeline/bookmarks";
export { ListIndex, ListTimeline, loadLists, loadListTimeline } from "./timeline/lists";
export { FindPeople, loadPeople } from "./people/people";
export { Search } from "./search/search";
export { ListMembers, loadListMembers } from "./timeline/list-members";
export { loadTagTimeline, TagTimeline } from "./timeline/tag";
export { useWelcomeTitle, Welcome, type WelcomeProps, type WelcomeSpecimen } from "./welcome/welcome";
export {
  loadStaffEntry,
  loadStaffFacts,
  staffEntries,
  staffEntryPath,
  staffHref,
  staffRootPath,
  useStaffEntries,
  useStaffEntry,
  useStaffFacts,
  useStaffWaiting,
  type StaffAddress,
  type StaffArea,
  type StaffEntry,
  type StaffEntryProps,
  type StaffFacts,
  type StaffSearch,
} from "./staff/entries";
export { queueHref, queueOf, ReportQueueBeside, ReportQueueScreen, reportHref } from "./staff/reports/queue";
export { ReportScreen, useReportTitle, type ReportScreenProps } from "./staff/reports/report";
export { DomainActionScreen, useDomainActionTitle } from "./staff/domains/actions";
export { NewDomainActionScreen, SuspendConfirm, type NewDomainActionProps } from "./staff/domains/action-form";
export { DomainAllowsScreen } from "./staff/domains/allowed";
export { AuditSentence } from "./staff/domains/audit";
export { DomainImportScreen } from "./staff/domains/import";
export {
  auditLogPath,
  domainActionPath,
  domainActionsPath,
  domainAllowsPath,
  domainImportPath,
  domainListPath,
  domainListsPath,
  newDomainActionPath,
  newDomainListPath,
  remoteServerPath,
  remoteServersPath,
} from "./staff/domains/parts";
export { RemoteServerScreen, serverDomainOf, type RemoteServerScreenProps } from "./staff/domains/servers";
export {
  DomainListScreen,
  DomainListsScreen,
  NewDomainListScreen,
  useDomainListTitle,
  type DomainListScreenProps,
} from "./staff/domains/subscriptions";
export { StaffMissing, StaffRoot } from "./staff/root";
export { StaffEntryScreen, useStaffTitle, type StaffEntryScreenProps } from "./staff/screen";
export { adminRolePath, adminRolesPath, NewRoleScreen, RoleScreen } from "./staff/server/roles";
export { AnnouncementScreen, NewAnnouncementScreen } from "./staff/server/announcement";
export { WebhookScreen } from "./staff/server/webhook";
export { adminWebhookPath, adminWebhooksPath, NewWebhookScreen } from "./staff/server/webhooks";
export { WebhookSecretSpecimen } from "./staff/server/webhook-form";
export { EmojiScreen, NewEmojiScreen } from "./staff/server/emoji-item";
export { adminEmojiPath, adminEmojiRequestsPath, adminEmojisPath, newAdminEmojiPath } from "./staff/server/emoji-parts";
export { AdminEmojiRequestsScreen } from "./staff/server/emoji-requests";
export {
  CustomEmojiSettings,
  EmojiRequestScreen,
  loadCustomEmojiSettings,
  OwnEmojiScreen,
  useEmojiRequestTitle,
  useOwnEmojiTitle,
} from "./settings/custom-emojis";
export { AdminDocumentScreen, loadAdminDocument, useAdminDocumentTitle } from "./staff/documents";
export {
  LoginProviderScreen,
  loadLoginProviders,
  NewLoginProviderScreen,
  useLoginProviderTitle,
} from "./staff/login-providers";
export { loadOAuthHosts, OAuthHostsScreen } from "./staff/oauth-apps";
export { loadRelays, NewRelayScreen, RelayScreen, useRelayTitle } from "./staff/relays";
export { loadRules, NewRuleScreen, RuleScreen } from "./staff/rules";
export type { AdminSubProps } from "./staff/settings/parts";
export { AdminSettingsTopicScreen, loadAdminSettingsTopic, useAdminSettingsTopicTitle } from "./staff/settings/screen";
export { loadTranslationSettings, TranslationSettingsScreen } from "./staff/settings/translation";
export { loadUnderOperations } from "./staff/operations/entries";
export { errorGroupPath, ErrorGroupScreen, loadError, useErrorGroupTitle } from "./staff/operations/errors";
export {
  JobListScreen,
  jobPath,
  jobQueuePath,
  JobScreen,
  jobsListPath,
  loadJob,
  loadJobList,
  useJobListTitle,
  useJobTitle,
} from "./staff/operations/jobs";
export {
  LinkCardRuleScreen,
  loadLinkCardDomains,
  NewLinkCardRuleScreen,
  useLinkCardRuleTitle,
} from "./staff/operations/link-cards";
export { loadUXScreen, UXMeasurementsScreen } from "./staff/operations/ux";
export { ThemeDraftScreen, useThemeDraftTitle } from "./staff/theme/draft";
export { ClosureAnnouncementScreen, useClosureAnnouncementTitle } from "./staff/closure/announce";
export { ClosureCloseScreen, loadClosureClose, useClosureCloseTitle } from "./staff/closure/close";
export { loadClosure } from "./staff/closure/screen";
export { usePassed } from "./staff/closure/passed";
export { loadTheme } from "./staff/theme/screen";
export {
  accountPagePath,
  AccountScreen,
  useModeratedAccountTitle,
  type AccountPage,
  type AccountScreenProps,
} from "./staff/accounts/account";
export { AccountActionScreen, useAccountActionTitle, type AccountActionScreenProps } from "./staff/accounts/actions";
export { appealHref, AppealScreen, useAppealTitle, type AppealScreenProps } from "./staff/accounts/appeals";
export { loadUnderEntry, useUnderTitle } from "./staff/accounts/entries";
export {
  HookAppealScreen,
  HookAppealsScreen,
  hookAppealsPath,
  HookDecisionScreen,
  useHookAppealTitle,
  useHookDecisionTitle,
} from "./staff/accounts/hooks";
export { NewInviteScreen } from "./staff/accounts/invites";
export {
  ImportSignupBlocksScreen,
  NewSignupBlockScreen,
  SignupBlockScreen,
  signupBlockPath,
  signupBlocksPath,
  useSignupBlockTitle,
} from "./staff/accounts/signup-blocks";

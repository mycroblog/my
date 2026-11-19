// SPDX-License-Identifier: AGPL-3.0-only

// The operations of the client's screens that the specs use, each found by role and accessible name
// (restrict/e2e-queries): a spec says what a person does and sees, and these parts say how the screen shows it.
//
// An action that writes returns only once the server has answered the write (write.ts). The screen shows a write at
// once, before the answer, so asserting what it shows and then reloading or navigating is the flaky pattern: the
// reload aborts the unanswered request and the server drops the write.

export { AccountData } from "./account-data";
export { AccountNote } from "./account-note";
export { AccountStatus } from "./account-status";
export { Apps } from "./apps";
export { ArticleText } from "./article";
export { ArticleEditor, type Upload } from "./article-editor";
export { Composer } from "./composer";
export { Consent } from "./consent";
export { conversationRead, Conversations } from "./conversations";
export { Drafts } from "./drafts";
export { EmojiPicker } from "./emoji-picker";
export { EventForm, EventPage } from "./events";
export { countLink, Follows } from "./follows";
export { Frame, Menu, type MenuPlace, type Place } from "./frame";
export { Hashtag } from "./hashtag";
export { FilterEditor, foldedBy, KeywordFilters } from "./keyword-filters";
export { Invites } from "./invites";
export { ListIndex } from "./lists";
export { LoginProvider } from "./login-provider";
export { Moderation } from "./moderation";
export { streamOpened } from "./stream";
export { NotificationSettings } from "./notification-settings";
export { FollowRequests, Notifications, WaitingRequests } from "./notifications";
export { ownProfileHeading, Profile } from "./profile";
export { ProfileForm, ProfileSettings } from "./profile-settings";
export { Reactors } from "./reactors";
export { Relationships } from "./relationships";
export { ReportPanel, type ReportReason } from "./report";
export { ForgotPassword, PasswordReset } from "./reset";
export { Security, TwoFactorSetup, virtualAuthenticator } from "./security";
export { DisplaySettings, Landing, Settings } from "./settings";
export { signInAs, SignIn, type Passkey } from "./sign-in";
export { SignUp } from "./sign-up";
export { Thread } from "./thread";
export { VerifyEmail } from "./verify";
export { connectedPeers, keepPeerConnections, openPeers, receivedAudio, VoiceRoom, VoiceRoomOpen } from "./voice";
export { Welcome } from "./welcome";
export { Notice } from "./notice";
export { bookmarks, homeTimeline, localTimeline, Post, Timeline } from "./timeline";

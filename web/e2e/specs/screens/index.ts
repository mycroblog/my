// SPDX-License-Identifier: AGPL-3.0-only

// The operations of the client's screens that the specs use, each found by role and accessible name
// (restrict/e2e-queries): a spec says what a person does and sees, and these parts say how the screen shows it.
//
// An action that writes returns only once the server has answered the write (write.ts). The screen shows a write at
// once, before the answer, so asserting what it shows and then reloading or navigating is the flaky pattern: the
// reload aborts the unanswered request and the server drops the write.

export { AccountNote } from "./account-note";
export { HomeAnnouncements } from "./announcements";
export { ArticleText } from "./article";
export { ArticleEditor, holdPreviews, holdProcessing, processedIn, type Upload } from "./article-editor";
export { Composer } from "./composer";
export { Consent } from "./consent";
export {
  Conversation,
  conversationRead,
  Conversations,
  failingReads,
  heldWrite,
  nextReplyTarget,
} from "./conversations";
export { Drafts } from "./drafts";
export { EmojiPicker } from "./emoji-picker";
export {
  applyTheme,
  CjkFonts,
  EmojiFonts,
  selfHostedCjk,
  selfHostedNoto,
  selfHostedNotoSans,
  type CjkRegion,
  type DrawnWith,
} from "./fonts";
export { countLink, Follows } from "./follows";
export { Frame, Menu, type MenuPlace, type Place } from "./frame";
export { Hashtag } from "./hashtag";
export { ListIndex, ListScreen } from "./lists";
export { streamOpened } from "./stream";
export { Landing, type LandingPlace } from "./landing";
export { MediaViewer } from "./media";
export { Notifications, WaitingRequests } from "./notifications";
export { heldAccountReads, Profile } from "./profile";
export { Reactors } from "./reactors";
export { signInAs, SignIn, type Passkey } from "./sign-in";
export { Thread } from "./thread";
export { Notice } from "./notice";
export { bookmarks, homeTimeline, line, localTimeline, Post, Timeline } from "./timeline";

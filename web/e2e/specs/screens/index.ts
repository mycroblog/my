// SPDX-License-Identifier: AGPL-3.0-only

// The operations of the client's screens that the specs use, each found by role and accessible name
// (restrict/e2e-queries): a spec says what a person does and sees, and these parts say how the screen shows it.
//
// An action that writes returns only once the server has answered the write (write.ts). The screen shows a write at
// once, before the answer, so asserting what it shows and then reloading or navigating is the flaky pattern: the
// reload aborts the unanswered request and the server drops the write.

export { Composer } from "./composer";
export { Consent } from "./consent";
export { Frame, type Place } from "./frame";
export { Notifications } from "./notifications";
export { Profile } from "./profile";
export { signInAs, SignIn, type Passkey } from "./sign-in";
export { Thread } from "./thread";
export { homeTimeline, Post, Timeline } from "./timeline";

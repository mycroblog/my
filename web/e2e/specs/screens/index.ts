// SPDX-License-Identifier: AGPL-3.0-only

// The operations of the client's screens that the specs use, each found by role and accessible name
// (restrict/e2e-queries): a spec says what a person does and sees, and these parts say how the screen shows it.

export { Composer } from "./composer";
export { Consent } from "./consent";
export { Frame, type Place } from "./frame";
export { Notifications } from "./notifications";
export { Profile } from "./profile";
export { signInAs, SignIn, unusedCode, type Passkey } from "./sign-in";
export { Thread } from "./thread";
export { homeTimeline, Post, Timeline } from "./timeline";

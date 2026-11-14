// SPDX-License-Identifier: AGPL-3.0-only

// What the browser, the PWA and the OS can do, and their differences (ADR-0025 §2, ADR-0028 §17). Only this package
// reads navigator and the platform's globals (restrict/platform-globals refuses them elsewhere); the others ask it
// whether something can be done, and hold the screen for when it cannot.

export { isOnline, watchOnline } from "./connection";
export { readDeviceValue, writeDeviceValue } from "./device-values";
export { namesCommandKey } from "./keyboard";
export { watchKeyboardInset } from "./keyboard-inset";
export { browserLanguages } from "./languages";
export { oneTimeCodeField } from "./one-time-code";
export {
  createPasskey,
  getPasskey,
  PasskeyError,
  passkeyUsernameField,
  supportsPasskeyAutofill,
  supportsPasskeys,
  type PasskeyFailure,
  type PasskeyJSON,
} from "./passkeys";
export { joinTabGroup, type TabGroup } from "./tabs";
export { supportsViewTransitions } from "./transitions";

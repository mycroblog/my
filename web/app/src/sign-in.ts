// SPDX-License-Identifier: AGPL-3.0-only

// How the app signs in (ADR-0090 §16): as the web client of the shell's configuration, at the server of its client
// API, coming back to /login/callback on the page's own origin, with the start kept in the tab's sessionStorage.

import type { SignInOptions } from "@my/app-data";
import type { RuntimeConfig } from "./config";

/** The path of the app's sign-in, which the signed-out shell links to. */
export const signInPath = "/login";

/** The path of the sign-in's callback (internal/platform, WebClientCallbackPath). */
const callbackPath = "/login/callback";

/** The sign-in of the app in this page. */
export const signInOptions = (config: RuntimeConfig): SignInOptions => ({
  server: config.apiOrigin,
  clientId: config.clientId,
  redirectUri: new URL(callbackPath, location.origin).href,
  storage: sessionStorage,
});

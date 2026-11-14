// SPDX-License-Identifier: AGPL-3.0-only

// How the app signs in (ADR-0090 §16): as the web client of the shell's configuration, at the server of its client
// API, coming back to /login/callback on the page's own origin, with the start kept in the tab's sessionStorage.

import type { SignInOptions } from "@my/app-data";
import type { RuntimeConfig } from "./config";

/** The path of the app's sign-in, which the signed-out shell links to. */
export const signInPath = "/login";

/** The server's registration page (internal/interface/authpages, PathSignUp), which the landing page links to. */
export const signUpPath = "/auth/sign_up";

/**
 * Whether `href` is one of the server's own pages (its auth pages), which a link leaves the app for with a page load,
 * not through the router.
 */
export const isServerPage = (href: string) => href.startsWith("/auth/");

/** The app's sign-in, coming back to the app's path `returnTo` once signed in. */
export const signInReturningTo = (returnTo: string) => `${signInPath}?return_to=${encodeURIComponent(returnTo)}`;

/** The path of the sign-in's callback (internal/platform, WebClientCallbackPath). */
const callbackPath = "/login/callback";

/** The sign-in of the app in this page. */
export const signInOptions = (config: RuntimeConfig): SignInOptions => ({
  server: config.apiOrigin,
  clientId: config.clientId,
  redirectUri: new URL(callbackPath, location.origin).href,
  storage: sessionStorage,
});

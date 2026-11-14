// SPDX-License-Identifier: AGPL-3.0-only

// The features of the web client (ADR-0025 §2–§4), a directory each: what a screen of the app places, built from
// @my/app-components and fed by @my/app-data. A feature imports no other feature; the app wires them together.

export { parseAuthPage, type AuthPage, type ScreenName } from "./auth/data";
export { AuthScreen } from "./auth/screen";
export { HomeTimeline, loadHomeTimeline } from "./timeline/home";

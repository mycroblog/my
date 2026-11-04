// SPDX-License-Identifier: AGPL-3.0-only
// Where the web client runs: in a browser, or as the Tauri app (ADR-0028), whose WebView loads the same build. The app
// shows no way to subscribe to the supporter plans (ADR-0072 §14, web-client/09-app-shell.md §9 "Tauri のアプリ"); this module is
// the one flag that tells the two apart.

/**
 * Whether the client runs as the Tauri app: Tauri 2 sets `window.isTauri` to true in its WebView before any script
 * of the page runs, which is what `isTauri()` of `@tauri-apps/api` reads. It does not change while the page is open.
 */
export function isTauriApp(): boolean {
  return (globalThis as { isTauri?: unknown }).isTauri === true;
}

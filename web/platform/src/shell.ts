// SPDX-License-Identifier: AGPL-3.0-only

// Whether the web client runs inside a native app's shell (ADR-0028 §5, §17): the Tauri 2 app that wraps its UI on
// mobile, whose WebView marks the page's global object (`isTauri`). The app offers no purchase there, nor any way to
// one (ADR-0072 §14, ADR-0162 §3).

/** Whether the page runs in the native app's shell rather than a browser. */
export function inNativeShell(scope: object = globalThis): boolean {
  return Reflect.get(scope, "isTauri") === true;
}

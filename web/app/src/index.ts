// SPDX-License-Identifier: AGPL-3.0-only

// The app's screens for a page other than its entry (the catalog's preview of them, ADR-0168): the router and the
// root to render under an I18nProvider, and the keymap the catalog's parts run under. The app's own entry is main.tsx.

export { AppRoot } from "./app";
export { keymap } from "./keymap";
export { createAppRouter } from "./router";

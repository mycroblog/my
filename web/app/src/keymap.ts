// SPDX-License-Identifier: AGPL-3.0-only

// The one keymap of the web client (ADR-0025 §4): which keys run which action, by the action's ID, which AppRoot
// gives ShortcutProvider. The keys follow other fediverse clients, and none is one the browser keeps (Ctrl or ⌘ with
// N, T or W), which the provider refuses. timeline.next and timeline.previous are bound by the column of
// @my/app-components, compose.open by the app's frame. An action with no keys is declared and not yet bound
// (notifications.read_all, which the notifications of @my/app-features declare).

import type { Keymap } from "@my/design-system";

export const keymap = {
  "shortcuts.help": { keys: ["?"] },
  "compose.open": { keys: ["n"] },
  "timeline.next": { keys: ["j", "ArrowDown"] },
  "timeline.previous": { keys: ["k", "ArrowUp"] },
  "post.reply": { keys: ["r"] },
  "post.boost": { keys: ["b"] },
  "post.favourite": { keys: ["f"] },
  "post.more": { keys: ["m"] },
  "notifications.read_all": { keys: [] },
} as const satisfies Keymap;

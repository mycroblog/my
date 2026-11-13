// SPDX-License-Identifier: AGPL-3.0-only

// The one keymap of the web client (ADR-0025 §4): which keys run which action, by the action's ID. It is the app's;
// until @my/app exists (#2883) the catalog holds it, and it moves as it is to @my/app's src/keymap.ts, which places
// ShortcutProvider with it. The keys follow other fediverse clients, and none is one the browser keeps (Ctrl or ⌘
// with N, T or W), which the provider refuses. timeline.next and timeline.previous are bound by the column (#2989).

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
} as const satisfies Keymap;

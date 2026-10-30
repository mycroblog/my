// SPDX-License-Identifier: AGPL-3.0-only
// The emoji feature (FR-101, FR-111; ADR-0091, ADR-0051 §4): the emoji picker of a text field (the post form, the edit
// of a post, the profile), which puts a picked Unicode emoji or custom emoji's `:shortcode:` into the field, with the
// emoji used often on this browser and the skin tone chosen here; and the same picker for the emoji to react to a post
// with (ADR-0091 §7, ADR-0052 §5), or to choose one emoji such as a voice chat room's icon (FR-923); and the emoji that the completion after ":" in those fields offers (ADR-0092 §2). The admin screen of the emoji (features/admin) marks the list stale.
// Routes and other features use it through this file only (web-client/README.md A3).
export { EmojiPickerButton } from "./components/EmojiPickerButton";
export { ReactionPickerButton } from "./components/ReactionPickerButton";
/** A button that opens the same picker to choose one emoji, such as the icon of a voice chat room (FR-923). */
export { EmojiChoiceButton } from "./components/EmojiChoiceButton";
/**
 * The emoji that the completion after ":" offers (ADR-0092 §2): custom and Unicode emoji matched by name, those used
 * often on this browser first, a Unicode emoji in the skin tone chosen here.
 */
export { type EmojiSuggestion, useEmojiSuggestions } from "./hooks/emojiSuggestions";
/** The shortcodes of the local custom emoji, which the highlight layer of a text field marks (ADR-0092 §4). */
export { useCustomEmojiImages, useCustomEmojiShortcodes } from "./hooks/customEmojiShortcodes";
export type { ReactionChoice } from "./hooks/useReactionPicker";

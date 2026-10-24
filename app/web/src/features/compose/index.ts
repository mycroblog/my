// SPDX-License-Identifier: AGPL-3.0-only
// The compose feature: the new-post form with its draft, visibility, content warning, language, counter,
// images with descriptions, replies, and quotes (FR-101 to FR-105, FR-110, FR-114). Other features and routes use it
// through this file only (web-client/README.md A3): the layout places ComposeProvider and ComposeEntry, the navigation opens the sheet with useOpenCompose, and a
// reply or quote button calls the function from useReply or useQuote. The form's parts that editing a post reuses (the status
// feature's edit dialog, FR-109) are here too: the image descriptions, the language choices, and the count.
export { AttachmentList } from "./components/AttachmentList";
export { ComposeEntry, ComposeProvider, useOpenCompose } from "./components/Compose";
/** The text fields that complete mentions, hashtags and emoji (ADR-0092 §5): the edit dialog and the profile use them too. */
export { CompletionTextArea, CompletionTextField } from "./components/CompletionFields";
/** Remembers the hashtags of a post being sent, which the completion after "#" ranks first (ADR-0092 §1). */
export { rememberTags } from "./hooks/recentTags";
export { type LanguageOption, languageOptions } from "./hooks/languages";
export { countCharacters } from "./hooks/policy";
export { type AttachmentView } from "./hooks/useAttachments";
export { quotePolicies, quotePolicyDescription, quotePolicyLabel } from "./hooks/quotePolicyMessages";
export { useQuote, useReply } from "./hooks/useCompose";
export { useFormatRemaining } from "./hooks/useFormatRemaining";

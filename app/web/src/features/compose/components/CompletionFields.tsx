// SPDX-License-Identifier: AGPL-3.0-only
// The text fields that complete mentions, hashtags and emoji as they are typed (ADR-0092 §3, §5): the text of a post
// (the compose form and the edit dialog) and the bio and field values of the profile complete all three, the content
// warning and the display name emoji only. An option shows an account's avatar, display name (its custom emoji as
// images) and handle; a hashtag and how many posts carry it; an emoji's image or character and its shortcode or name.
import {
  Avatar,
  CustomEmoji,
  EmojiText,
  Text,
  TextAreaAutocomplete,
  type TextAreaProps,
  TextFieldAutocomplete,
  type TextFieldProps,
} from "@my/ui";
import { useT } from "../../../lib/i18n";
import { imageSources } from "../../../lib/images";
import { type CompletionKinds, type CompletionSuggestion, useTextCompletion } from "../hooks/useTextCompletion";

export type CompletionTextAreaProps = TextAreaProps & {
  /** What the field completes: mentions, hashtags and emoji (`all`), or emoji only. */
  completes: CompletionKinds;
};

/** A TextArea that completes what `completes` says. */
export function CompletionTextArea({ completes, ...props }: CompletionTextAreaProps) {
  const completion = useTextCompletion(completes);
  return (
    <TextAreaAutocomplete<CompletionSuggestion>
      {...props}
      {...completion}
      renderSuggestion={(suggestion) => <SuggestionContent suggestion={suggestion} />}
    />
  );
}

export type CompletionTextFieldProps = TextFieldProps & {
  /** What the field completes: mentions, hashtags and emoji (`all`), or emoji only. */
  completes: CompletionKinds;
};

/** A TextField (one line) that completes what `completes` says. */
export function CompletionTextField({ completes, ...props }: CompletionTextFieldProps) {
  const completion = useTextCompletion(completes);
  return (
    <TextFieldAutocomplete<CompletionSuggestion>
      {...props}
      {...completion}
      renderSuggestion={(suggestion) => <SuggestionContent suggestion={suggestion} />}
    />
  );
}

/** The content of an option. */
function SuggestionContent({ suggestion }: { suggestion: CompletionSuggestion }) {
  const t = useT();
  switch (suggestion.kind) {
    case "account": {
      const { account } = suggestion;
      const name = account.display_name || account.username;
      return (
        <>
          <Avatar sources={imageSources(account.avatar)} name={name} seed={account.id} decorative size="xs" />
          <Text dir="auto" weight="medium" wrap="truncate" shrink>
            <EmojiText text={name} emojis={account.emojis} />
          </Text>
          {/* Visually the gap of the row; it keeps the parts apart in the option's accessible name. */}{" "}
          <Text size="sm" tone="muted" wrap="truncate" shrink>
            {suggestion.value}
          </Text>
        </>
      );
    }
    case "tag":
      return (
        <>
          <Text dir="auto" wrap="truncate" shrink>
            {suggestion.value}
          </Text>{" "}
          {suggestion.tag.statusesCount !== undefined && (
            <Text size="sm" tone="muted" numeric>
              {t("client:compose.completion.tag_posts", { count: suggestion.tag.statusesCount })}
            </Text>
          )}
        </>
      );
    case "emoji": {
      const { emoji } = suggestion;
      return emoji.kind === "custom" ? (
        <>
          {/* The image's alternative text is the shortcode, which the text after it says already. */}
          <Text aria-hidden>
            <CustomEmoji emoji={emoji.emoji} />
          </Text>
          <Text wrap="truncate" shrink>
            {emoji.value}
          </Text>
        </>
      ) : (
        <>
          <Text aria-hidden>{emoji.value}</Text>{" "}
          <Text size="sm" tone="muted" wrap="truncate" shrink>
            {emoji.emoji.name}
          </Text>
        </>
      );
    }
  }
}

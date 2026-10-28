// SPDX-License-Identifier: AGPL-3.0-only
import { Box, Button, Hint, PostContent, serverSanitizedHtml, Spinner, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import type { ReplyTarget } from "../hooks/useComposeForm";

export type ReplyContextProps = {
  replyTo: Exclude<ReplyTarget, { status: "none" }>;
  onCancel: () => void;
  isDisabled?: boolean;
  /** Whether the post is replied to (the default) or quoted (FR-114). */
  kind?: "reply" | "quote";
};

const messages = {
  reply: {
    label: "client:compose.reply.label",
    unavailable: "client:compose.reply.unavailable",
    cancel: "client:compose.reply.cancel",
  },
  quote: {
    label: "client:compose.quote.label",
    unavailable: "client:compose.quote.unavailable",
    cancel: "client:compose.quote.cancel",
  },
} as const;

/**
 * The post being replied to or quoted, above the form: its author and the start of its text (or only its
 * content warning, which the reply does not open), and a button that drops it from the draft.
 */
export function ReplyContext({ replyTo, onCancel, isDisabled = false, kind = "reply" }: ReplyContextProps) {
  const t = useT();
  const m = messages[kind];
  return (
    <Stack gap="2" border="md" background="sunken" padding="3">
      {replyTo.status === "loading" && <Spinner size="sm" label={t("client:shell.loading")} />}
      {replyTo.status === "error" && <Hint>{t(m.unavailable)}</Hint>}
      {replyTo.status === "ready" && (
        <>
          <Text as="p" size="sm" weight="medium">
            {t(m.label, { name: replyTo.post.account.display_name || `@${replyTo.post.account.acct}` })}
          </Text>
          {replyTo.post.spoiler_text === "" ? (
            <PostContent
              html={serverSanitizedHtml(replyTo.post.content)}
              lang={replyTo.post.language}
              size="sm"
              lineClamp={3}
            />
          ) : (
            <Text as="p" dir="auto" lang={replyTo.post.language} size="sm" wrap="anywhere">
              {replyTo.post.spoiler_text}
            </Text>
          )}
        </>
      )}
      <Box self="start">
        <Button size="sm" variant="ghost" onPress={onCancel} isDisabled={isDisabled}>
          {t(m.cancel)}
        </Button>
      </Box>
    </Stack>
  );
}

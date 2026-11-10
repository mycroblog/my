// SPDX-License-Identifier: AGPL-3.0-only
// The page that a follow or reply button of another server opens (the Activity Intents of WebFinger, FEP-3b86,
// web-client/screens/intent.md): what the other server asks for, the address it sent, and one button that looks it up
// and opens the profile to follow or the post to reply to. A failure shows next to the button.
import { Button, Cluster, InlineError, Stack, Text } from "@my/ui";
import { useT } from "../../../lib/i18n";
import { type ActivityIntentOptions, useActivityIntent } from "../hooks/useActivityIntent";

export type ActivityIntentProps = ActivityIntentOptions;

export function ActivityIntent(props: ActivityIntentProps) {
  const t = useT();
  const intent = useActivityIntent(props);
  const follow = props.kind === "follow";
  return (
    <Stack gap="3">
      <p>{t(follow ? "client:intent.follow.description" : "client:intent.reply.description")}</p>
      {/* The address comes from the other server: it is shown as it came, in its own direction. */}
      <Text as="p" mono wrap="anywhere" dir="auto">
        {props.target}
      </Text>
      <Cluster gap="3">
        {/* A pending button keeps its focus and tells screen readers it is busy (React Aria Button, isPending). */}
        <Button variant="primary" isPending={intent.isPending} onPress={intent.open}>
          {t(follow ? "client:intent.follow.submit" : "client:intent.reply.submit")}
        </Button>
      </Cluster>
      {intent.error !== null && <InlineError>{intent.error}</InlineError>}
    </Stack>
  );
}

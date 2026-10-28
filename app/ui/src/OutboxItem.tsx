// SPDX-License-Identifier: AGPL-3.0-only
// A post waiting to be sent: its start, and either the progress of sending it or the reason it is still here and its actions.
import type { ReactNode } from "react";
import { Cluster } from "./Cluster";
import { Spinner } from "./Spinner";
import { Stack } from "./Stack";
import { InlineError, Text } from "./Text";

export type OutboxItemProps = {
  /** The post's text, cut after three lines. */
  text: string;
  /** The post's language (BCP 47). */
  lang?: string | undefined;
  /** While the post is being sent: what is in progress ("Sending"). The item is then `aria-busy`. */
  sendingLabel?: string | undefined;
  /** When it is not being sent: why it is still here ("Could not send: offline"). */
  error?: string | undefined;
  /** The actions (`Button size="sm"`) shown with `error`: retry, edit, discard. */
  children?: ReactNode;
};

/** A `li` for a `<Stack as="ul" gap="2">`: a framed row of the post's text and its state. */
export function OutboxItem({ text, lang, sendingLabel, error, children }: OutboxItemProps) {
  return (
    <Stack as="li" gap="2" border="md" padding="3" aria-busy={sendingLabel !== undefined}>
      <Text
        as="p"
        dir="auto"
        {...(lang !== undefined && { lang })}
        lineClamp={3}
        whitespace="preWrap"
        wrap="anywhere"
        tone="muted"
      >
        {text}
      </Text>
      {sendingLabel !== undefined ? (
        <Cluster as="p" align="center" gap="2" tone="muted" textSize="sm">
          <Spinner size="sm" label={sendingLabel} />
          <span aria-hidden="true">{sendingLabel}</span>
        </Cluster>
      ) : (
        <>
          {error !== undefined && <InlineError>{error}</InlineError>}
          <Cluster gap="2">{children}</Cluster>
        </>
      )}
    </Stack>
  );
}

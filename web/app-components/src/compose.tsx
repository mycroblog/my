// SPDX-License-Identifier: AGPL-3.0-only

// The parts of the composer that know a post: the post replied to and the quoted post, above the text, and the poll
// as it is written, a raised card whose options are headed by the mark they will be answered with, in the look of
// the poll a post carries (poll.tsx). Every word is the caller's. They are composed of the design system's parts
// alone, laid out and painted by their props (ADR-0025 §4).

import { Box, Button, ChoiceField, Icon, IconButton, Pin, Row, Spoken, Stack } from "@my/design-system";
import type { ReactNode } from "react";

/** The post replied to, above the text: its byline and the start of its text, muted, cut after two lines. */
export function ComposerReply({ byline, note, children }: { byline: ReactNode; note: string; children: ReactNode }) {
  return (
    <Stack gap="entry-line" tone="muted">
      {byline}
      {/* Muted, also where what people write sets its own color. */}
      <Box lines={2} tone="all-muted">
        {children}
      </Box>
      <Spoken>{note}</Spoken>
    </Stack>
  );
}

/**
 * The quoted post, above the text, as the post will carry it (an `Embed`, whose first line keeps `clear` of the
 * button), with its remove at its top end.
 */
export function ComposerQuote({
  removeLabel,
  onRemove,
  children,
}: {
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  return (
    <Box anchor>
      {children}
      <Pin as="span" top="composer-field-block" end="entry-line">
        <IconButton icon="close" label={removeLabel} onPress={onRemove} />
      </Pin>
    </Box>
  );
}

/** The poll: a raised card under its caption and its remove; its options, then its settings. */
export function PollCard({
  title,
  removeLabel,
  onRemove,
  children,
}: {
  title: string;
  removeLabel: string;
  onRemove: () => void;
  children: ReactNode;
}) {
  return (
    <Stack as="fieldset" shrink gap="entry-line" surface="raised" radius="control" pad="embed">
      <Spoken as="legend">{title}</Spoken>
      <Row align="center" justify="between" bleedBlock="composer-inset" bleedEnd="icon-button-inline">
        <Row as="span" inline align="center" gap="entry-caption" text="caption" tone="muted">
          <Icon name="poll" place="caption" />
          {title}
        </Row>
        <IconButton icon="close" label={removeLabel} onPress={onRemove} />
      </Row>
      {children}
    </Stack>
  );
}

/** The options of a poll, together on the page's surface inside the card; the last line adds one. */
export function PollOptions({
  add,
  children,
}: {
  add?: { label: string; onPress: () => void } | undefined;
  children: ReactNode;
}) {
  return (
    <Stack
      surface="page"
      radius="control"
      padY="composer-inset"
      padStart="control-timeline-inline"
      padEnd="composer-inset"
    >
      {children}
      {add && (
        <Row as="span" bleedStart="control-timeline-inline">
          <Button kind="quiet" size="timeline" onPress={add.onPress}>
            <Row as="span" inline align="center" gap="entry-caption">
              <Icon name="add" place="action" />
              {add.label}
            </Row>
          </Button>
        </Row>
      )}
    </Stack>
  );
}

/** An option of a poll, headed by the mark it will be answered with; its remove at its end, where it may go. */
export function PollOption({
  value,
  onChange,
  label,
  multiple,
  maxLength,
  remove,
  error,
}: {
  value: string;
  onChange: (value: string) => void;
  label: string;
  /** Several may be chosen: a checkbox's square, else a radio's ring. */
  multiple: boolean;
  maxLength: number;
  remove?: { label: string; onPress: () => void } | undefined;
  /** What is wrong with it, right under it. */
  error?: string | undefined;
}) {
  return (
    <ChoiceField
      value={value}
      onChange={onChange}
      label={label}
      multiple={multiple}
      maxLength={maxLength}
      remove={remove}
      error={error}
    />
  );
}

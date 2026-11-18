// SPDX-License-Identifier: AGPL-3.0-only

// The head of a profile: the banner, 3:1, on the raised surface when there is none and from edge to edge on a narrow
// column; the avatar over its lower edge, half on it, ringed by the page's surface, with the reader's actions at the
// other end of its row, as far from the banner's edge as from the avatar; the name with its marks over the handle and
// its badges; then the rest, a profile's gap apart (the bio, the fields, when it joined and its counts, lines of the
// account's state, the featured hashtags). As a `summary`, inside another screen, its banner is a strip and its name
// no heading. Its parts: the fields, the line of counts, the reader's private note (a
// line that opens its editor in place), and a list of hashtags. It is composed of the design system's parts alone,
// laid out and painted by their props (ADR-0025 §4).

import {
  BannerHead,
  BareTextArea,
  Box,
  Button,
  ButtonPair,
  ChipLink,
  Icon,
  PressRow,
  QuietLink,
  Row,
  Spoken,
  Stack,
  useLocale,
} from "@my/design-system";
import type { ReactNode } from "react";

export type ProfileHeadProps = {
  /** The id of the name, which names the head. */
  nameId: string;
  /** The banner's image; the raised surface without one. */
  banner?: string | undefined;
  /** The avatar, at the preview's size. */
  avatar: ReactNode;
  /** What the reader may do, at the end of the avatar's row. */
  actions?: ReactNode;
  /** The display name. */
  name: ReactNode;
  /** What follows the name on its line (a locked account's lock, a bot's badge). */
  marks?: ReactNode;
  /** The handle, as `@user@domain`. */
  handle: string;
  /** What follows the handle on its line (follows you, the role badges). */
  badges?: ReactNode;
  /** The rest of the head, one part under another. */
  children?: ReactNode;
  /**
   * The head shown small inside another screen (the reader's own profile, in their settings): the banner is a strip
   * within the head, and the name is a line of it, no heading of the page.
   */
  summary?: boolean | undefined;
};

export function ProfileHead({
  nameId,
  banner,
  avatar,
  actions,
  name,
  marks,
  handle,
  badges,
  children,
  summary = false,
}: ProfileHeadProps) {
  const { direction } = useLocale();
  return (
    <BannerHead banner={banner} lead={avatar} end={actions} labelledBy={nameId} dir={direction} strip={summary}>
      <Stack gap="hair">
        <Row
          as={summary ? "p" : "h2"}
          wrap
          align="center"
          gap="screen-name-gap"
          text={summary ? "body" : "title"}
          weight={summary ? "strong" : undefined}
        >
          <bdi id={nameId}>{name}</bdi>
          {marks}
        </Row>
        <Row as="p" wrap align="center" gap="screen-name-gap" text="caption" tone="muted">
          <bdi dir="ltr">{handle}</bdi>
          {badges}
        </Row>
      </Stack>
      {children}
    </BannerHead>
  );
}

/** A profile's fields: each name, muted and small, over its value. */
export function FieldList({ items }: { items: readonly { name: ReactNode; value: ReactNode }[] }) {
  return (
    <Stack as="dl" gap="screen-field-gap">
      {items.map((item, i) => (
        <Stack key={i} as="div">
          <Box as="dt" text="caption" tone="muted">
            {item.name}
          </Box>
          <dd>{item.value}</dd>
        </Stack>
      ))}
    </Stack>
  );
}

/** A line of a profile's facts: when it joined (text) and its counts (`MetaCount`). */
export function MetaLine({ children }: { children: ReactNode }) {
  return (
    <Row as="p" wrap gapX="screen-meta-gap" gapY="screen-tab-gap" text="caption" tone="muted">
      {children}
    </Row>
  );
}

/** A count of a profile's line: its label, then its value in the text's color; a link when it opens a list. */
export function MetaCount({ label, value, href }: { label: string; value: string; href?: string | undefined }) {
  const body = (
    <>
      {label}{" "}
      <Box as="strong" text="body" weight="strong" tone="text">
        {value}
      </Box>
    </>
  );
  return href === undefined ? (
    <span>{body}</span>
  ) : (
    <QuietLink href={href} tone="muted">
      {body}
    </QuietLink>
  );
}

/**
 * The reader's private note on an account, as a line of its profile that opens its editor: the note, or with none,
 * `placeholder` (what adding one does), muted. `label` names the line for assistive technology before what it shows.
 */
export function NoteLine({
  note: text,
  placeholder,
  label,
  onPress,
}: {
  note: string;
  placeholder: string;
  label: string;
  onPress?: (() => void) | undefined;
}) {
  return (
    <PressRow flow="row" bleed="inline" gap="entry-caption" text="caption" tone="muted" onPress={onPress}>
      <Row as="span" inline line rigid align="center">
        <Icon name="note" place="caption" />
      </Row>
      <Box as="span" grow keeps="spaces" breaks="phrase" tone={text === "" ? undefined : "text"} dir="auto">
        {text !== "" && <Spoken>{label} </Spoken>}
        {text === "" ? placeholder : text}
      </Box>
    </PressRow>
  );
}

/** The note's editor in the line's place: a box as tall as the note, then Cancel and Save. */
export function NoteEditor({
  label,
  value,
  onChange,
  maxLength,
  cancel,
  save,
  onCancel,
  onSave,
  busy = false,
}: {
  /** The box's name, for assistive technology. */
  label: string;
  value: string;
  onChange: (value: string) => void;
  maxLength: number;
  cancel: string;
  save: string;
  onCancel: () => void;
  onSave: () => void;
  busy?: boolean | undefined;
}) {
  return (
    <Stack gap="screen-profile-gap">
      {/* It opens on the reader's press, in the line's place. */}
      <BareTextArea label={label} value={value} onChange={onChange} maxLength={maxLength} autoFocus />
      <ButtonPair>
        <Button kind="quiet" size="timeline" onPress={onCancel} isDisabled={busy}>
          {cancel}
        </Button>
        <Button kind="primary" size="timeline" busy={busy} onPress={onSave}>
          {save}
        </Button>
      </ButtonPair>
    </Stack>
  );
}

/** The hashtags a profile features, under a heading: each a link on the raised surface, its count muted. */
export function TagList({
  heading,
  items,
}: {
  heading: string;
  items: readonly { name: string; href: string; count: string }[];
}) {
  return (
    <Stack gap="screen-name-gap">
      <Box as="h3" text="caption" weight="normal" tone="muted">
        {heading}
      </Box>
      <Row as="ul" wrap plain gap="screen-name-gap">
        {items.map((item) => (
          <li key={item.name}>
            <ChipLink href={item.href} count={item.count}>
              <bdi>#{item.name}</bdi>
            </ChipLink>
          </li>
        ))}
      </Row>
    </Stack>
  );
}

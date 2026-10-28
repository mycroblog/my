// SPDX-License-Identifier: AGPL-3.0-only
// An open voice chat room in a list (ADR-0100 §8), in the look of the room's stage: the "Live" pill with how many are
// in the room and how long it has been open (and "Joined" while the user is in its call), the room's name as a
// heading that links to its page, the stacked avatars of its speakers (the host first) beside the host, and, in the
// full form, its description and hashtags, each a link to the hashtag's page as on the stage. It is the one look of
// an open room wherever one shows: the list page, the third column, a hashtag's page, and a profile. The name, the
// description and the hashtags are the host's own text in any language (dir="auto"). The `card` surface is a raised
// card of a page's list; the `plain` surface is a row of a panel that is a card already (the third column), which
// never holds another card.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { AvatarStack, type AvatarStackItem } from "./AvatarStack";
import { Card } from "./Card";
import type { HeadingLevel } from "./EmptyState";
import { Heading } from "./Heading";
import { CheckIcon } from "./Icons";
import { Link } from "./Link";
import { LivePill } from "./LivePill";
import { StatusBadge } from "./StatusBadge";
import { Tag } from "./Tag";

const voiceRoomCard = tv({
  slots: {
    base: "flex min-w-0 flex-col",
    state: "flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-fg-muted",
    name: "font-serif",
    // The avatars keep their size; the host's name takes the room left and wraps inside it.
    people: "flex min-w-0 items-center gap-2 text-sm",
    host: "min-w-0 wrap-anywhere",
    role: "shrink-0 text-fg-muted",
    description: "line-clamp-3 text-sm whitespace-pre-line text-fg",
    tags: "flex flex-wrap gap-1",
  },
  variants: {
    full: {
      true: { base: "gap-3", name: "text-lg" },
      false: { base: "gap-2" },
    },
  },
});

/** A hashtag of the room: its text with the `#`, and its page. */
export type VoiceRoomCardTag = { label: string; href: string };

export type VoiceRoomCardProps = {
  /** The room's name, the host's text. */
  name: string;
  /** The room's page. */
  href: string;
  /** The heading's level: one below the heading of the list. */
  headingLevel?: HeadingLevel;
  /** The word on the pill of an open room ("Live"). */
  liveLabel: string;
  /** How many are in the room and how long it has been open, formatted ("12 people · 38 min"). */
  stats: string;
  /**
   * The word of the badge that marks the room of the user's call ("Joined"); absent for any other room. A user in a
   * call finds it in every list, and does not join another room by mistake.
   */
  joinedLabel?: string | undefined;
  /** The speakers, the host first; the card stacks the first few. They only decorate: `host` and `stats` say who is in. */
  speakers: readonly AvatarStackItem[];
  /** The host's name, a link to their profile; on a profile, the account's name. */
  host: ReactNode;
  /** The word after the host's name ("Host"); on a profile, the account's role ("Host", "Speaker"). */
  hostLabel: string;
  /** The room's description, the host's text; empty or absent for none. Not shown on the `plain` surface. */
  description?: string | undefined;
  /** The room's hashtags, each a link to its page. Not shown on the `plain` surface. */
  tags?: readonly VoiceRoomCardTag[] | undefined;
  /** The accessible name of the list of hashtags ("Hashtags"). */
  tagsLabel?: string;
  /** `card` (default) for a page's list, `plain` for a row inside a card (the third column). Both are an `li`. */
  surface?: "card" | "plain";
};

/** One open room as an `li`: put several in a `ul` (`Stack as="ul"`). */
export function VoiceRoomCard({
  name,
  href,
  headingLevel = 2,
  liveLabel,
  stats,
  joinedLabel,
  speakers,
  host,
  hostLabel,
  description,
  tags,
  tagsLabel,
  surface = "card",
}: VoiceRoomCardProps) {
  const full = surface === "card";
  const slots = voiceRoomCard({ full });
  const content = (
    <div className={slots.base()}>
      <div className={slots.state()}>
        <LivePill label={liveLabel} />
        {joinedLabel !== undefined && <StatusBadge icon={<CheckIcon />} label={joinedLabel} tone="success" />}
        <span>{stats}</span>
      </div>
      <Heading level={headingLevel} size="subsection" wrap="anywhere">
        <Link href={href} variant="standalone">
          <span dir="auto" className={slots.name()}>
            {name}
          </span>
        </Link>
      </Heading>
      <div className={slots.people()}>
        <AvatarStack avatars={speakers} max={full ? 4 : 3} size={full ? "sm" : "xs"} />
        <span className={slots.host()}>{host}</span>
        <span className={slots.role()}>{hostLabel}</span>
      </div>
      {full && description !== undefined && description !== "" && (
        <p dir="auto" className={slots.description()}>
          {description}
        </p>
      )}
      {full && tags !== undefined && tags.length > 0 && (
        <ul className={slots.tags()} aria-label={tagsLabel}>
          {tags.map((tag) => (
            <Tag key={tag.href} as="li" variant="pill" dir="auto">
              <Link href={tag.href} variant="standalone">
                {tag.label}
              </Link>
            </Tag>
          ))}
        </ul>
      )}
    </div>
  );
  if (!full) return <li>{content}</li>;
  return (
    <Card as="li" padding="4">
      {content}
    </Card>
  );
}

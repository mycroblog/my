// SPDX-License-Identifier: AGPL-3.0-only
// An open voice chat room in a list (ADR-0100 §8): its name as a heading that links to the room's page, its host, how
// many people are in it, and, in the full form, its description and hashtags. The name, the description and the
// hashtags are the host's own text in any language (dir="auto"). The `card` surface is a raised card of a page's list;
// the `plain` surface is a row of a panel that is a card already (the third column), which never holds another card.
import type { ReactNode } from "react";
import { tv } from "tailwind-variants";
import { Card } from "./Card";
import type { HeadingLevel } from "./EmptyState";
import { Heading } from "./Heading";
import { Link } from "./Link";
import { Tag } from "./Tag";

const voiceRoomCard = tv({
  slots: {
    base: "flex min-w-0 flex-col gap-2",
    meta: "flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm text-fg-muted",
    description: "line-clamp-3 text-sm whitespace-pre-line text-fg",
    tags: "flex flex-wrap gap-1",
  },
});

export type VoiceRoomCardProps = {
  /** The room's name, the host's text. */
  name: string;
  /** The room's page. */
  href: string;
  /** The heading's level: one below the heading of the list. */
  headingLevel?: HeadingLevel;
  /** The host, as an account item (`AccountItem` with `layout="inline"`). */
  host: ReactNode;
  /** How many people are in the room, formatted ("3 people"). */
  participants: string;
  /** The room's description, the host's text; empty or absent for none. Not shown on the `plain` surface. */
  description?: string | undefined;
  /** The room's hashtags with their `#`. Not shown on the `plain` surface. */
  tags?: readonly string[] | undefined;
  /** The accessible name of the list of hashtags ("Hashtags"). */
  tagsLabel?: string;
  /** `card` (default) for a page's list, `plain` for a row inside a card (the third column). Both are an `li`. */
  surface?: "card" | "plain";
};

/** One open room as an `li`: put several in a `ul` (`Stack as="ul"`). */
export function VoiceRoomCard({ name, href, headingLevel = 2, host, participants, description, tags, tagsLabel, surface = "card" }: VoiceRoomCardProps) {
  const slots = voiceRoomCard();
  const full = surface === "card";
  const content = (
    <div className={slots.base()}>
      <Heading level={headingLevel} size="subsection" wrap="anywhere">
        <Link href={href} variant="standalone">
          <span dir="auto">{name}</span>
        </Link>
      </Heading>
      <div className={slots.meta()}>
        {host}
        <span>{participants}</span>
      </div>
      {full && description !== undefined && description !== "" && (
        <p dir="auto" className={slots.description()}>
          {description}
        </p>
      )}
      {full && tags !== undefined && tags.length > 0 && (
        <ul className={slots.tags()} aria-label={tagsLabel}>
          {tags.map((tag) => (
            <Tag key={tag} as="li" variant="pill" dir="auto">
              {tag}
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

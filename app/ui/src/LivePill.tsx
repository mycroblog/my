// SPDX-License-Identifier: AGPL-3.0-only
// The coral pill that marks a voice chat room as open now ("Live"), at the head of the room's page and of its card.
// The word is the pill's text, so it is read as it is shown.
import { tv } from "tailwind-variants";

const livePill = tv({
  base: "inline-flex shrink-0 items-center rounded-full bg-accent-fill px-2 py-1 text-xs leading-none font-extrabold tracking-wide text-accent-fill-fg",
});

export type LivePillProps = {
  /** The word on the pill ("Live"). */
  label: string;
};

/** The "Live" pill of an open room. */
export function LivePill({ label }: LivePillProps) {
  return <span className={livePill()}>{label}</span>;
}

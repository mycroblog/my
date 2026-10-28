// SPDX-License-Identifier: AGPL-3.0-only
// The coral pill that marks a voice chat room as open now ("Live"), at the head of the room's page and of its card.
// The word is the pill's text, so it is read as it is shown. While the user's call to the room reconnects, the pill
// turns from filled coral to an outline (`interrupted`): the room is still open, but the user does not hear it now;
// the call's connection badge beside it says why.
import { tv } from "tailwind-variants";

const livePill = tv({
  base: "inline-flex shrink-0 items-center rounded-full border px-2 py-1 text-xs leading-none font-extrabold tracking-wide",
  variants: {
    interrupted: {
      false: "border-transparent bg-accent-fill text-accent-fill-fg",
      true: "border-dashed border-accent bg-transparent text-accent",
    },
  },
  defaultVariants: { interrupted: false },
});

export type LivePillProps = {
  /** The word on the pill ("Live"). */
  label: string;
  /** Whether the user's call to the room is reconnecting: the pill shows as a dashed outline. */
  interrupted?: boolean;
};

/** The "Live" pill of an open room. */
export function LivePill({ label, interrupted = false }: LivePillProps) {
  return (
    <span className={livePill({ interrupted })} data-interrupted={interrupted ? "" : undefined}>
      {label}
    </span>
  );
}

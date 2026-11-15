// SPDX-License-Identifier: AGPL-3.0-only

// The suggestions for the word at the caret of the composer's text (ADR-0092 §3): accounts for `@`, hashtags for `#`,
// emoji for `:`, as a listbox under the caret's line, across the text's width, on the page's surface with a shadow as a
// menu stands. Each option is one row of a menu's height: an account's face, its name and its muted handle on one line;
// a tag and its count at the row's end; an emoji and its name. Where the row is short, the name and the handle's user
// part give up their width, never the handle's server, which tells same-named people apart. The text keeps the focus:
// the active option is the text's `aria-activedescendant`, stepped a surface up as a menu's hovered item, and a press
// on an option does not take the focus from the text. While the suggestions are asked for, or when none came, one
// muted line stands in their place; the field announces it (`FieldSuggest`'s `announce`), as it does their count. Every
// word is the caller's.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { motion } from "@my/design-tokens";
import { Avatar } from "./avatar";

export type Suggestion = (
  | {
      kind: "account";
      /** The account's display name, its custom emoji drawn. */
      name: ReactNode;
      /** `@user` for a local account, `@user@domain` for another server's. */
      handle: string;
      avatar?: string | undefined;
      /** The name in words, for the avatar's initial. */
      initialOf: string;
    }
  | {
      kind: "hashtag";
      /** The tag, without its `#`. */
      name: string;
      /** How many posts carry it, in words ("12 posts"). */
      uses: string;
    }
  | {
      kind: "emoji";
      /** `:shortcode:` of a custom emoji, or a Unicode emoji's name. */
      name: string;
      /** A custom emoji's image. */
      image?: string | undefined;
      /** A Unicode emoji itself. */
      text?: string | undefined;
    }
) & {
  /** The option's name for assistive technology, where its words alone do not read well ("Sam, @sam"). */
  label?: string | undefined;
};

const list = tv({
  slots: {
    panel: [
      "box-border grid max-h-[calc(8*var(--control-form-height)+2*var(--menu-padding))] overflow-y-auto rounded-overlay",
      "bg-surface p-menu-padding font-sans text-body text-text shadow-overlay",
    ],
    option: [
      "box-border flex h-control-form min-w-0 cursor-default items-center gap-menu-icon-gap overflow-hidden",
      "rounded-control px-control-timeline-inline hover:bg-surface-raised aria-selected:bg-surface-raised",
    ],
    lead: "inline-flex shrink-0 items-center justify-center",
    // The name gives up its width first, down to a few letters; then the handle's user part, down to its start.
    name: "min-w-[min(4em,30%)] shrink truncate font-label",
    handle: "flex min-w-0 shrink-[4] text-muted-text",
    user: "min-w-[3em] truncate",
    server: "shrink-0 whitespace-nowrap",
    text: "flex min-w-0 flex-1 items-baseline gap-entry-caption",
    end: "ms-auto shrink-0 ps-entry-caption text-caption text-muted-text tabular-nums",
    emoji: "size-icon-action text-[length:var(--icon-action)] leading-none object-contain",
    line: "m-0 flex h-control-form items-center gap-entry-caption px-control-timeline-inline text-caption text-muted-text",
    spinner: [
      "box-border size-icon-caption rounded-full border-spinner border-solid border-spinner-track border-t-current",
      "animate-spin",
      motion.spin.reduced === "still" ? "motion-reduce:animate-none" : "",
    ],
  },
});

export type SuggestionListProps = {
  /** The listbox's id, which the text's `aria-controls` names; an option's is `<id>-<index>`. */
  id: string;
  /** The listbox's name. */
  label: string;
  items: readonly Suggestion[];
  /** The index of the active option. */
  active: number;
  onChoose: (index: number) => void;
  /** Asked for and not here yet (after a moment), or none came: the line that says so, in their place. */
  status?: { kind: "loading" | "empty"; text: string } | undefined;
};

/** The suggestions for the word at the caret, or the line that says they are coming or that none came. */
export function SuggestionList({ id, label, items, active, onChoose, status }: SuggestionListProps) {
  const s = list();
  if (status !== undefined)
    return (
      <div className={s.panel()}>
        <p className={s.line()}>
          {status.kind === "loading" && <span className={s.spinner()} aria-hidden="true" />}
          {status.text}
        </p>
      </div>
    );
  return (
    <div id={id} className={s.panel()} role="listbox" aria-label={label}>
      {items.map((item, i) => (
        <div
          key={i}
          id={`${id}-${String(i)}`}
          className={s.option()}
          role="option"
          aria-selected={i === active}
          aria-label={item.label}
          // The text keeps the focus, and its caret.
          onMouseDown={(e) => {
            e.preventDefault();
          }}
          onClick={() => {
            onChoose(i);
          }}
        >
          <Row item={item} s={s} />
        </div>
      ))}
    </div>
  );
}

/** `@user@domain` cut before its second `@`: the user part, and the server part (none for a local account). */
const handleParts = (handle: string): [string, string] => {
  const at = handle.indexOf("@", 1);
  return at < 0 ? [handle, ""] : [handle.slice(0, at), handle.slice(at)];
};

function Row({ item, s }: { item: Suggestion; s: ReturnType<typeof list> }) {
  switch (item.kind) {
    case "account": {
      const [user, server] = handleParts(item.handle);
      return (
        <>
          <span className={s.lead()}>
            <Avatar name={item.initialOf} src={item.avatar} size="row" decorative />
          </span>
          <span className={s.text()}>
            <span className={s.name()} dir="auto">
              {item.name}
            </span>
            <span className={s.handle()} dir="ltr">
              <span className={s.user()}>{user}</span>
              {server !== "" && <span className={s.server()}>{server}</span>}
            </span>
          </span>
        </>
      );
    }
    case "hashtag":
      return (
        <>
          <span className={s.name()} dir="auto">
            #{item.name}
          </span>
          {/* Apart from the tag in the option's words, as in its name. */} <span className={s.end()}>{item.uses}</span>
        </>
      );
    case "emoji":
      return (
        <>
          <span className={s.lead()}>
            {item.image !== undefined ? (
              <img className={s.emoji()} src={item.image} alt="" />
            ) : (
              <span className={`${s.emoji()} inline-flex items-center justify-center`} aria-hidden="true">
                {item.text}
              </span>
            )}
          </span>
          <span className={s.name()} dir="auto">
            {item.name}
          </span>
        </>
      );
  }
}

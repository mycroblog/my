// SPDX-License-Identifier: AGPL-3.0-only

// The versions of an edited entry (a post's edit history), every word the caller's: the versions newest first, a
// feed entry's room apart, each headed by its label (Current, Original) and its time over what it holds. What a
// version changed from the one before it is marked inline, the same way for its words and its images: added on a
// neutral tint (the text's color at a small share, apart from the warm surfaces and from the coral of links, and with no
// underline, which reads as a link), removed in the muted text struck through (an image: muted and crossed out). Each
// says what it is in text only screen readers hear (the roles of <ins> and <del> take no name), so the marks read
// without colour.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

const history = tv({
  slots: {
    list: "m-0 grid list-none gap-entry-room p-0",
    version: "grid grid-cols-[minmax(0,1fr)] gap-entry",
    head: "flex min-w-0 flex-wrap items-baseline gap-x-entry-name gap-y-0 leading-entry",
    label: "font-strong",
    time: "text-caption text-muted-text",
    text: "m-0 whitespace-pre-wrap [overflow-wrap:anywhere]",
    added: "rounded-[2px] bg-diff-added text-text no-underline [box-decoration-break:clone]",
    removed: "text-muted-text line-through decoration-1",
    replaced: "me-[0.3em]",
    media: "m-0 flex list-none flex-wrap gap-media p-0",
    tile: "relative box-border rounded-control",
    image: "block h-media-preview-height w-media-preview-width rounded-control object-cover",
    // An added image: on the tint, which shows around it as a frame.
    tileAdded: "bg-diff-added p-[0.25rem]",
    // A removed image: muted, and crossed out by a line of the muted text from corner to corner.
    tileRemoved: [
      "opacity-50 after:absolute after:inset-0 after:rounded-control",
      "after:bg-[linear-gradient(to_top_right,transparent_calc(50%-1px),var(--color-muted-text)_calc(50%-1px),var(--color-muted-text)_calc(50%+1px),transparent_calc(50%+1px))]",
    ],
  },
});

/** The versions, newest first, each a `Version`. */
export function VersionList({ label, children }: { label: string; children: ReactNode }) {
  return (
    <ol aria-label={label} className={history().list()}>
      {children}
    </ol>
  );
}

export type VersionProps = {
  /** Current, Edit, Original. */
  label: string;
  /** When it was made, as the caller writes it (a `<time>`). */
  time: ReactNode;
  /** The language of what it holds. */
  lang?: string | undefined;
  /** What it holds: its content warning, its text, its images. */
  children: ReactNode;
};

/** One version: its label and its time, then what it holds. */
export function Version({ label, time, lang, children }: VersionProps) {
  const s = history();
  return (
    <li className={s.version()}>
      <div className={s.head()}>
        <span className={s.label()}>{label}</span>
        <span className={s.time()}>{time}</span>
      </div>
      <div className={s.version()} lang={lang}>
        {children}
      </div>
    </li>
  );
}

/** What a run of a version changed from the version before it. */
export type DiffKind = "same" | "added" | "removed";

/** A run of a version's text: the same as before, added, or removed. */
export type DiffPart = { text: string; kind: DiffKind };

/** What screen readers hear around the marks, in the reader's language (`lang`), not the version's. */
export type DiffMarks = {
  lang?: string | undefined;
  /** Said before and after a run of added words. */
  added: { start: string; end: string };
  /** Said before and after a run of removed words. */
  removed: { start: string; end: string };
  /** Said before an added or removed image's description. */
  addedImage: string;
  removedImage: string;
};

/** Text only screen readers hear, apart from the words around it. */
function Heard({ text, lang }: { text: string; lang?: string | undefined }) {
  return (
    <span className="sr-only" lang={lang}>
      {` ${text} `}
    </span>
  );
}

/** A run's text as it is: DiffText's `render` when none is given. */
const plain = (text: string): ReactNode => text;

/**
 * A version's text with what it changed marked: added words on the tint, removed ones struck through and muted, each
 * run between the start and end `marks` screen readers hear. `render` draws a run's text (its emoji).
 */
export function DiffText({
  parts,
  marks,
  render = plain,
}: {
  parts: readonly DiffPart[];
  marks: DiffMarks;
  render?: ((text: string) => ReactNode) | undefined;
}) {
  const s = history();
  return (
    <p className={s.text()}>
      {parts.map((p, i) =>
        p.kind === "same" ? (
          <span key={i}>{render(p.text)}</span>
        ) : p.kind === "added" ? (
          <ins key={i} className={s.added()}>
            <Heard text={marks.added.start} lang={marks.lang} />
            {render(p.text)}
            <Heard text={marks.added.end} lang={marks.lang} />
          </ins>
        ) : (
          <del
            key={i}
            // The words that took their place follow a space apart.
            className={parts[i + 1]?.kind === "added" ? `${s.removed()} ${s.replaced()}` : s.removed()}
          >
            <Heard text={marks.removed.start} lang={marks.lang} />
            {render(p.text)}
            <Heard text={marks.removed.end} lang={marks.lang} />
          </del>
        ),
      )}
    </p>
  );
}

/** An image of a version, as a small preview: kept, added or removed. */
export type DiffImage = { id: string; src: string; alt: string; kind: DiffKind };

/**
 * A version's images as small previews, what it changed marked as its words are: an added image on the tint, a removed
 * one muted and crossed out; screen readers hear `marks.addedImage` or `marks.removedImage` before each one's `alt`.
 */
export function DiffMedia({ items, marks }: { items: readonly DiffImage[]; marks: DiffMarks }) {
  const s = history();
  if (items.length === 0) return null;
  return (
    <ul className={s.media()}>
      {items.map((m) => {
        const image = <img className={s.image()} src={m.src} alt={m.alt} />;
        return (
          <li
            key={`${m.kind}-${m.id}`}
            className={
              m.kind === "added"
                ? `${s.tile()} ${s.tileAdded()}`
                : m.kind === "removed"
                  ? `${s.tile()} ${s.tileRemoved()}`
                  : s.tile()
            }
          >
            {m.kind === "added" ? (
              <ins>
                <Heard text={marks.addedImage} lang={marks.lang} />
                {image}
              </ins>
            ) : m.kind === "removed" ? (
              <del>
                <Heard text={marks.removedImage} lang={marks.lang} />
                {image}
              </del>
            ) : (
              image
            )}
          </li>
        );
      })}
    </ul>
  );
}

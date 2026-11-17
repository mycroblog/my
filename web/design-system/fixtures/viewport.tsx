// SPDX-License-Identifier: AGPL-3.0-only
// restrict/viewport-breakpoints: only the frame of a page decides by the viewport (ADR-0025 §4).
// web/devtools/src/layout-parts.js names Page of this file as a page frame, and Column as a layout part that is none.
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";

const page = tv({
  slots: {
    root: "min-h-dvh tablet:flex tablet:justify-center desktop:gap-shell-gap-wide max-tablet:flex-col",
    side: "hidden tablet:sticky min-[40rem]:block wide:w-shell-aside",
  },
});

// The frame of a page reads the breakpoints, in its classes and in a media query.
export function Page({ side, children }: { side: ReactNode; children: ReactNode }) {
  const s = page();
  const narrow = window.matchMedia("(max-width: 40rem)").matches || window.innerWidth < 640;
  return (
    <div className={s.root()}>
      {!narrow && <nav className={s.side()}>{side}</nav>}
      <main className="tablet:px-shell-edge">{children}</main>
    </div>
  );
}

// A layout part that is no frame lays itself out by its container.
const column = tv({
  base: [
    "@container grid @reply:grid-cols-2",
    "tablet:grid-cols-2", // error: viewport tablet:grid-cols-2
  ],
});

export function Column({ children }: { children: ReactNode }) {
  return <div className={column()}>{children}</div>;
}

// Every other part takes the frame's decision as a prop. The reader's preferences are no viewport.
const note = tv({
  base: [
    "text-body motion-reduce:animate-none dark:text-text pointer-coarse:h-icon-button-touch data-hovered:underline",
    "desktop:text-title", // error: viewport desktop:text-title
    "max-desktop:hidden", // error: viewport max-desktop:hidden
    "min-[40rem]:underline", // error: viewport min-[40rem]:underline
    "not-tablet:hidden", // error: viewport not-tablet:hidden
    "md:hidden", // error: viewport md:hidden
    "portrait:hidden", // error: viewport portrait:hidden
    "[@media(min-width:40rem)]:underline", // error: viewport [@media(min-width:40rem)]:underline
    "[@media(prefers-contrast:more)]:underline",
    "hover:tablet:underline", // error: viewport hover:tablet:underline
    "container", // error: viewport container
  ],
});

export function Note({ wide, children }: { wide: boolean; children: ReactNode }) {
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const landscape = matchMedia("(orientation: landscape)").matches; // error: viewport matchMedia(orientation)
  const roomy = window.matchMedia(`(min-width: ${40}rem)`).matches; // error: viewport matchMedia(min-width)
  const width = window.innerWidth; // error: viewport innerWidth
  const height = window.innerHeight;
  return (
    <p className={note()} data-wide={wide || roomy || landscape} data-reduced={reduced}>
      <span
        className="wide:font-label" // error: viewport wide:font-label
      >
        {children} {width} {height}
      </span>
    </p>
  );
}

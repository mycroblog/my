// SPDX-License-Identifier: AGPL-3.0-only
// restrict/layout-parts: only a layout part writes layout (ADR-0171 §3). web/devtools/src/layout-parts.js names Row
// and Shelf of this file as layout parts, and Gone, which is not here; every other part writes its own box alone.
// web/devtools/src/restrictions.test.js lints it; the lint of the package leaves it out.

import type { ReactNode } from "react"; // error: layout-part Gone
import { tv } from "tailwind-variants/lite";

// A layout part places and spaces: by its classes, a class written in place, a constant and a style.
const row = tv({ base: "relative flex flex-wrap items-center gap-chip-spacing overflow-hidden" });
const apart = "ms-auto min-w-0 max-w-full";

export function Row({ width, children }: { width: number; children: ReactNode }) {
  return (
    <div className={row()} style={{ width }}>
      <span className={apart}>{children}</span>
      <span className="absolute inset-0 z-1">{children}</span>
    </div>
  );
}

const shelf = tv({
  slots: {
    root: "@container grid grid-cols-[minmax(0,1fr)] gap-feed",
    item: "min-w-0 @reply:ms-screen-reply-step",
    shared: "flex", // error: layout flex
    unread: "grid", // error: layout grid
  },
  variants: { wide: { true: { root: "w-full", item: "col-span-2" } } },
});

export function Shelf({
  wide,
  first,
  edge,
  children,
}: {
  wide: boolean;
  first: ReactNode;
  edge: string;
  children: ReactNode;
}) {
  const s = shelf({ wide });
  return (
    <div className={s.root()}>
      <div
        // A class comes in under a name that says so, in a layout part too: its callers are other declarations.
        className={edge} // error: layout edge
      />
      <div className={s.item()}>{first}</div>
      <div className={s.shared()}>{children}</div>
    </div>
  );
}

// A part's own box, and what is no layout: a padding and a fixed size from tokens, colour, type, radius, what shows
// and hides. A variant's value is no class.
const tag = tv({
  slots: {
    base: [
      "box-border h-chip min-h-control-form w-avatar-inline px-chip-inline py-0 size-spinner aspect-square",
      "rounded-control bg-surface-raised text-caption whitespace-nowrap text-text data-hovered:bg-surface-raised-hover",
      "hidden sr-only contents content-[''] inset-ring-2 translate-y-press [overflow-wrap:anywhere] [--gap:1rem]",
    ],
    glyph: "inline-flex", // error: layout inline-flex
    label: [
      "hover:block", // error: layout hover:block
      "items-center!", // error: layout items-center!
      "gap-chip-gap", // error: layout gap-chip-gap
      "-mx-chip-inline", // error: layout -mx-chip-inline
      "[&_p]:m-0", // error: layout [&_p]:m-0
      "w-full", // error: layout w-full
      "w-1/2", // error: layout w-1/2
      "min-w-0", // error: layout min-w-0
      "max-w-avatar-post", // error: layout max-w-avatar-post
      "h-[1lh]", // error: layout h-[1lh]
      "px-[3px]", // error: layout px-[3px]
      "p-2", // error: layout p-2
      "sticky", // error: layout sticky
      "top-0", // error: layout top-0
      "z-10", // error: layout z-10
      "truncate", // error: layout truncate
      "overflow-x-auto", // error: layout overflow-x-auto
      "align-middle", // error: layout align-middle
      "@container", // error: layout @container
      "@reply:text-body", // error: layout @reply:text-body
      "[@container(min-width:20rem)]:text-body", // error: layout [@container(min-width:20rem)]:text-body
      "[grid-template-columns:1fr_auto]", // error: layout [grid-template-columns:1fr_auto]
    ],
  },
  variants: {
    kind: { flex: { base: "font-label" }, grid: { base: "font-strong" } },
    end: { true: { label: "ms-auto" } }, // error: layout ms-auto
  },
  compoundVariants: [{ kind: "flex", end: true, class: { glyph: "self-center" } }], // error: layout self-center
  defaultVariants: { kind: "flex" },
});
const lead = "grow"; // error: layout grow
const box = { display: "grid", padding: 4 }; // error: layout style.display

export function Tag({ kind, depth, children }: { kind: "flex" | "grid"; depth: number; children: ReactNode }) {
  const s = tag({ kind, end: depth > 0 });
  // A class the call of a slot adds, one a template adds, and one a template makes.
  const base = s.base({ class: "mt-entry" }); // error: layout mt-entry
  const glyph = `${s.glyph()} float-end`; // error: layout float-end
  const indent = `ms-[calc(${depth}*1rem)] ${lead}`; // error: layout ms-[calc(${…}*1rem)]
  return (
    <span className={base}>
      <span className={glyph}>{children}</span>
      <span className={depth > 0 ? indent : s.label()}>{children}</span>
      <span className={shelf().shared()} style={box}>
        {children}
      </span>
      <span
        style={{
          aspectRatio: "1",
          insetInlineStart: 0, // error: layout style.insetInlineStart
          maxWidth: "100%", // error: layout style.maxWidth
        }}
      />
    </span>
  );
}

// What a declaration gives an element as a class outside an attribute: in an object of props, in an assignment to
// one, in a variable given more after its declaration, through a function of the file, and in the `classList`.
const wide = (on: boolean) => (on ? "w-full" : "text-body"); // error: layout w-full
function joined(className: string, on: boolean) {
  let out = className;
  if (on) out += " font-label";
  return out;
}

export function Loose({ on, children }: { on: boolean; children: ReactNode }) {
  let c = "text-body";
  if (on) c += " ms-auto"; // error: layout ms-auto
  const props: { className?: string } = {};
  props.className = "grid"; // error: layout grid
  const more = { className: "flex-none" }; // error: layout flex-none
  return (
    <span {...props}>
      <span className={c}>{children}</span>
      <span className={wide(on)} {...more} />
      <span
        className={joined(
          "truncate", // error: layout truncate
          on,
        )}
        ref={(element) => {
          element?.classList.add("sticky"); // error: layout sticky
        }}
      />
    </span>
  );
}

// A class comes into a declaration under a name that says so, `className` or one that ends in `ClassName`: the lint
// reads what the callers give under that name. The state a component gives a function of classes is no class.
export function Named({
  className,
  innerClassName,
  look,
  children,
}: {
  className: string;
  innerClassName: (hovered: boolean) => string;
  look: string;
  children: ReactNode;
}) {
  const state = (hovered: boolean) => (hovered ? innerClassName(hovered) : className);
  return (
    <span className={className}>
      <span className={state(true)}>{children}</span>
      <span
        className={look} // error: layout look
      />
    </span>
  );
}

export function Taken(props: { tone: string; className: string }) {
  return (
    <span
      className={`${props.className} ${
        props.tone // error: layout tone
      }`}
    />
  );
}

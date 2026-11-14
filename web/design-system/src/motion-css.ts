// SPDX-License-Identifier: AGPL-3.0-only

// The motion of each kind of change of screen (motion.tsx), as the rules theme.ts adds to the stylesheet. A View
// Transition of a kind carries the kind as its type, and these rules move the element named `screen` (the frame's main
// column) by it: a push and a pop shift it along the line, mirrored in a right-to-left page; a modal raises the new
// screen over the old one; a replace crossfades them. The page around it, and the frame's fixed bars, stay. A shared
// element (Shared) moves from its place on the old screen to its place on the new one in the kind's time. With reduced
// motion nothing moves, even where a View Transition was started elsewhere. A modal over the page (the composer, the
// keyboard shortcuts) is no navigation: Motion moves it (modal.tsx).

type Rules = Record<string, Record<string, string> | Record<string, Record<string, string>>>;

/** The kinds that move, as View Transition types. */
const moving = ["push", "pop", "modal", "replace"] as const;

const during = (...kinds: string[]) => `:root:active-view-transition-type(${kinds.join(", ")})`;

/** The shift along the line toward the end (1) or the start (-1) of a line in the page's direction. */
const along = (sign: 1 | -1) => `translateX(calc(${sign} * var(--transition-shift) * var(--my-direction)))`;

/** A screen leaves fading out early, then shifts to its side; one enters from the other side, fading in late. */
const slide = (kind: "push" | "pop", leaveTo: 1 | -1): Rules => ({
  [`@keyframes my-${kind}-out`]: { "35%": { opacity: "0" }, to: { opacity: "0", transform: along(leaveTo) } },
  [`@keyframes my-${kind}-in`]: {
    from: { opacity: "0", transform: along(-leaveTo as 1 | -1) },
    "35%": { opacity: "0" },
  },
  [`${during(kind)}::view-transition-old(screen)`]: {
    animation: `var(--transition-${kind}) var(--transition-ease-exit) both my-${kind}-out`,
  },
  [`${during(kind)}::view-transition-new(screen)`]: {
    animation: `var(--transition-${kind}) var(--transition-ease-enter) both my-${kind}-in`,
  },
});

export const transitionRules: Rules = {
  ":root": { "--my-direction": "1" },
  ':root[dir="rtl"]': { "--my-direction": "-1" },
  // The page around the screen changes at once; the screen and the shared elements move in the kind's time.
  [`${during(...moving)}::view-transition-old(root), ${during(...moving)}::view-transition-new(root)`]: {
    animation: "none",
  },
  ...Object.fromEntries(
    moving.map((kind) => [
      `${during(kind)}::view-transition-group(*)`,
      {
        "animation-duration": `var(--transition-${kind})`,
        "animation-timing-function": "var(--transition-ease-enter)",
      },
    ]),
  ),
  // A screen that moves covers what is under it instead of adding its light to it; a replace crossfades.
  [`${during("push", "pop", "modal")}::view-transition-old(screen), ${during("push", "pop", "modal")}::view-transition-new(screen)`]:
    { "mix-blend-mode": "normal" },
  // A push leaves toward the start of the line and enters from its end; a pop goes back the other way.
  ...slide("push", -1),
  ...slide("pop", 1),
  "@keyframes my-rise-in": { from: { opacity: "0", transform: "translateY(var(--transition-rise))" } },
  [`${during("modal")}::view-transition-old(screen)`]: { animation: "none" },
  [`${during("modal")}::view-transition-new(screen)`]: {
    animation: "var(--transition-modal) var(--transition-ease-enter) both my-rise-in",
  },
  "@keyframes my-fade-in": { from: { opacity: "0" } },
  "@keyframes my-fade-out": { to: { opacity: "0" } },
  [`${during("replace")}::view-transition-old(screen)`]: {
    animation: "var(--transition-replace) linear both my-fade-out",
  },
  [`${during("replace")}::view-transition-new(screen)`]: {
    animation: "var(--transition-replace) linear both my-fade-in",
  },
  "@media (prefers-reduced-motion: reduce)": {
    "::view-transition-group(*), ::view-transition-old(*), ::view-transition-new(*)": { animation: "none !important" },
  },
};

// SPDX-License-Identifier: AGPL-3.0-only

// The backdrop of a modal (the keyboard shortcuts, the composer's dialog): the page behind it dimmed by the scrim and
// blurred by `--dialog-blur`, so that the reader's eye stays on the modal; a reader who asks for less transparency gets
// the dim alone. The dim and the blur fade in over a modal's time (motion-css.ts, my-backdrop-in), the one motion of the
// backdrop; the composer's dialog rises in the same time by its View Transition. With reduced motion it is there at once.
// The composer is a screen of its own on a phone and has a backdrop from `tablet` only. Tailwind reads the classes as
// they are written here, so each variant is spelled out, in a .tsx file, which styles.css scans.

/** The backdrop at every width. */
export const backdrop = [
  "bg-scrim backdrop-blur-[var(--dialog-blur)]",
  "[@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none",
  "motion-safe:animate-[my-backdrop-in_var(--transition-modal)_var(--transition-ease-enter)_both]",
].join(" ");

/** The backdrop from `tablet`, under which the modal is a screen of its own. */
export const backdropFromTablet = [
  "tablet:bg-scrim tablet:backdrop-blur-[var(--dialog-blur)]",
  "tablet:[@media(prefers-reduced-transparency:reduce)]:backdrop-blur-none",
  "tablet:motion-safe:animate-[my-backdrop-in_var(--transition-modal)_var(--transition-ease-enter)_both]",
].join(" ");

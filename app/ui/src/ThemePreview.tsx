// SPDX-License-Identifier: AGPL-3.0-only
import type { CSSProperties } from "react";
import { tv } from "tailwind-variants";

const themePreview = tv({
  slots: {
    root: "flex flex-col gap-3 rounded-lg border border-border bg-surface p-4 font-sans text-fg",
    card: "flex flex-col gap-2 rounded-md bg-surface-raised p-4 shadow-md",
    heading: "text-lg font-semibold",
    muted: "text-sm text-fg-muted",
    link: "text-accent underline underline-offset-2",
    button: "self-start rounded-md bg-accent px-4 py-2 font-medium text-accent-fg",
    danger: "text-danger",
  },
});

export type ThemePreviewProps = {
  /** The theme's overrides as custom properties (D10 allows React's `style` for them): the preview reads them like the app does. */
  tokens: CSSProperties;
  heading: string;
  text: string;
  muted: string;
  /** The look-alike of a link; the preview has nothing to operate. */
  link: string;
  /** The look-alike of a button. */
  button: string;
  danger: string;
};

/** How a theme looks: a surface with a card, text, a link, a button and an error, drawn with the overridden tokens. */
export function ThemePreview({ tokens, heading, text, muted, link, button, danger }: ThemePreviewProps) {
  const styles = themePreview();
  return (
    <div style={tokens} className={styles.root()}>
      <div className={styles.card()}>
        <p className={styles.heading()}>{heading}</p>
        <p>{text}</p>
        <p className={styles.muted()}>{muted}</p>
        <p className={styles.link()}>{link}</p>
        <p className={styles.button()}>{button}</p>
      </div>
      <p className={styles.danger()}>{danger}</p>
    </div>
  );
}

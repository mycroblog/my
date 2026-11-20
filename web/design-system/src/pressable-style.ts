// SPDX-License-Identifier: AGPL-3.0-only

// React Aria's usePress puts a `<style id="react-aria-pressable-style">` in the head of a document where a pressable
// mounts (`touch-action: pan-x pan-y pinch-zoom` on `[data-react-aria-pressable]`, so that a tap waits for no double
// tap), unless an element of that id is there already. The shell's policy (`style-src 'self'`, with no nonce) refuses
// an inline style, so the rule is in styles.css, and an app claims the id before it mounts anything.

/** The id React Aria's usePress gives the style it puts in the head (react-aria, usePress, STYLE_ID). */
const pressableStyleId = "react-aria-pressable-style";

/**
 * Puts an inert element of the pressables' style id in the head of `doc`, so that React Aria's usePress finds it and
 * writes no style of its own; styles.css holds the rule. An app calls it once, before it renders.
 */
export function claimPressableStyle(doc: Document = document): void {
  if (doc.getElementById(pressableStyleId) !== null) return;
  const marker = doc.createElement("template");
  marker.id = pressableStyleId;
  doc.head.append(marker);
}

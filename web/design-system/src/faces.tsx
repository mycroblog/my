// SPDX-License-Identifier: AGPL-3.0-only

// The faces of several people in a row, each a little over the one before: a conversation's people, the people a
// line is about. They are a picture of who, not a control: assistive technology hears the names beside them.

import type { ReactNode } from "react";
import { tv } from "tailwind-variants/lite";
import { Row } from "./layout";

const faces = tv({
  slots: {
    // The ring that cuts a face out of the one before takes the surface under the entry, hovered or not.
    root: [
      "[--face-cut-color:var(--color-surface)]",
      "in-data-surface-marked:[--face-cut-color:var(--color-surface-hover)]",
    ],
    face: "rounded-full shadow-face-cut",
  },
});

export type FacesProps = {
  /** The faces (avatars at a row's size), the first of them first. */
  children: readonly ReactNode[];
  /** The most that show; all of them when it is left out. */
  limit?: number | undefined;
};

/** The faces of several people as a strip of avatars in a row, each a little over the next so that every one stays recognisable. */
export function Faces({ children, limit }: FacesProps) {
  const s = faces();
  return (
    <Row as="span" inline rigid align="center" overlap="screen-face-tuck" className={s.root()} aria-hidden="true">
      {children.slice(0, limit).map((face, i) => (
        <Row key={i} as="span" inline rigid className={s.face()}>
          {face}
        </Row>
      ))}
    </Row>
  );
}

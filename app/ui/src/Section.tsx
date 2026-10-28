// SPDX-License-Identifier: AGPL-3.0-only
// A titled region of a screen below its `h1`: a `<section>` named by its `h2`, in the look of a region title
// (web-client/16-look-states-and-wording.md G5). Features build their regions from it and never write their own
// `section` with an `h2` (restrict/pattern in app/.oxlintrc.json), so every region has the same heading, space and surface.
import { type ReactNode, type Ref, type RefObject, useId } from "react";
import { Card } from "./Card";
import { Heading } from "./Heading";
import { Stack } from "./Stack";
import { Hint } from "./Text";

export type SectionProps = {
  /** The title: the `h2` that names the section. Required, so that no region goes untitled. */
  title: ReactNode;
  /** A note on the whole region under the title (`Hint`), which also describes the section (`aria-describedby`). */
  description?: ReactNode;
  /**
   * The surface: `plain` (the default) lays the region on the page; `card` on a raised card (a region of a detail page
   * or a pane); `band` with a rule under it and the inner space of a row (a notice at the top of a list); `rows` leaves
   * the content full-width for rows that bring their own padding, and gives the heading the rows' padding (G2).
   */
  surface?: "plain" | "card" | "band" | "rows";
  /** The heading's `id`, for a control in the region that refers to it (`aria-describedby`); generated when unset. */
  headingId?: string;
  /** With a ref, the heading takes focus from script (`tabindex="-1"`) when the region changes under it. */
  headingRef?: RefObject<HTMLHeadingElement | null>;
  /** The section element, for a region that takes focus itself when it appears (with `focusable`). */
  ref?: Ref<HTMLElement>;
  /**
   * Lets the section itself receive focus from script (`tabindex="-1"`), so that a screen reader reads its title and
   * description at once: for a region that appears after an action and must be read before anything else.
   */
  focusable?: boolean;
  /** Marks the region busy (`aria-busy`) while its content loads or uploads. */
  busy?: boolean;
  children: ReactNode;
};

/** A `<section>` named by its `h2`, with the space of a region's content between them. */
export function Section({
  title,
  description,
  surface = "plain",
  headingId,
  headingRef,
  ref,
  focusable = false,
  busy,
  children,
}: SectionProps) {
  const generatedId = useId();
  const descriptionId = useId();
  const id = headingId ?? generatedId;
  const heading = (
    <Heading level={2} ref={headingRef} id={id} focusable={headingRef !== undefined}>
      {title}
    </Heading>
  );
  const note = description !== undefined && <Hint id={descriptionId}>{description}</Hint>;
  const attributes = {
    ref,
    tabIndex: focusable ? -1 : undefined,
    "aria-labelledby": id,
    "aria-describedby": description === undefined ? undefined : descriptionId,
    "aria-busy": busy,
  };
  switch (surface) {
    case "card":
      return (
        <Card as="section" padding="6" {...attributes}>
          <Stack gap="3">
            {heading}
            {note}
            {children}
          </Stack>
        </Card>
      );
    case "rows":
      return (
        <Stack as="section" {...attributes}>
          <Stack gap="1" paddingX="4" paddingTop="4" paddingBottom="2">
            {heading}
            {note}
          </Stack>
          {children}
        </Stack>
      );
    case "band":
      return (
        <Stack as="section" gap="3" border="bottom" padding="4" {...attributes}>
          {heading}
          {note}
          {children}
        </Stack>
      );
    case "plain":
      return (
        <Stack as="section" gap="3" {...attributes}>
          {heading}
          {note}
          {children}
        </Stack>
      );
  }
}

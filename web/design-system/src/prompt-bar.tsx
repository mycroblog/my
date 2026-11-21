// SPDX-License-Identifier: AGPL-3.0-only

// A bar that opens a place to write (an event's board's composer, #3354): one control as a whole, as wide as the
// column that stretches it (a `Stack`), on the raised surface, the writer's avatar at the posts' size, the prompt on one line, cut at its end where the bar is narrower,
// and the glyph of writing at its end. It rests on the raised surface, takes the surface's hover and press shades and
// the press's shift, and the focus ring; it has no busy state, since what it opens opens at once. The prompt is its
// name for assistive technology.

import type { ReactNode } from "react";
import { Button as Press } from "react-aria-components";
import { Icon } from "./icons";
import { Box, Row } from "./layout";

export type PromptBarProps = {
  /** The writer's avatar, at the posts' size and decorative (`Avatar size="post" decorative`). */
  avatar: ReactNode;
  /** The prompt, on one line: the bar's name too. */
  children: string;
  onPress: () => void;
  /** Hears the bar take and lose the focus (what holds it hands the focus on when the bar goes). */
  onFocusChange?: ((focused: boolean) => void) | undefined;
};

export function PromptBar({ avatar, children, onPress, onFocusChange }: PromptBarProps) {
  return (
    <Row
      align="center"
      gap="entry-avatar"
      as={
        <Press
          onPress={onPress}
          onFocusChange={(focused) => {
            onFocusChange?.(focused);
          }}
          className="box-border cursor-pointer rounded-entry border-0 bg-surface-raised p-embed text-start font-sans text-body text-muted-text outline-hidden data-focus-visible:shadow-focus data-hovered:bg-surface-hover data-pressed:translate-y-press data-pressed:bg-surface-pressed"
        />
      }
    >
      {avatar}
      <Box grow shrink lines={1}>
        {children}
      </Box>
      <Icon name="compose" place="action" />
    </Row>
  );
}

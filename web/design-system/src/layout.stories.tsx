// SPDX-License-Identifier: AGPL-3.0-only

// The generic layout parts (layout.tsx), one story each: the part placing plain blocks, which stand for whatever a
// part of the design system gives it, with the props that say how. The blocks' own look (a tone, a padding, a size)
// is the story's; the placement is the part's alone.

import { rootPx, text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties, ReactNode } from "react";
import { Box, Column, Grid, Layers, Pin, Row, Stack, Sticky } from "./layout";

const rem = (px: number) => `${px / rootPx}rem`;

const type: CSSProperties = { fontSize: `${text.caption.size}rem`, lineHeight: text.caption.lineHeight };
const block: CSSProperties = {
  ...type,
  padding: `${rem(6)} ${rem(10)}`,
  borderRadius: "var(--radius-control)",
  background: "var(--color-surface-raised)",
  color: "var(--color-text)",
};
const quiet: CSSProperties = { ...block, background: "var(--color-surface-raised-pressed)" };
const frame: CSSProperties = {
  boxSizing: "border-box",
  maxWidth: rem(360),
  padding: rem(8),
  borderRadius: "var(--radius-overlay)",
  boxShadow: "inset 0 0 0 1px var(--color-surface-raised-pressed)",
};

const long = "A sentence long enough to run past the room its row leaves it, at any width a story is shown at";

/** What a part of the design system would give: a plain block, named by what it stands for. */
const Block = ({ children, style }: { children: ReactNode; style?: CSSProperties }) => (
  <div style={{ ...block, ...style }}>{children}</div>
);

const meta = { title: "Layout", parameters: { layout: "padded" } } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * One box and how it sits where it is put: in a row, the first keeps its size, the second takes the room left and
 * keeps its text to one line, and the third stands at the row's end.
 */
export const BoxInRow: Story = {
  name: "Box",
  render: () => (
    <div style={frame}>
      <Row align="center" gap="entry-caption">
        <Box rigid>
          <Block>Rigid</Block>
        </Box>
        <Box grow shrink lines={1}>
          <span style={type}>{long}</span>
        </Box>
        <Box rigid push="end">
          <Block style={quiet}>End</Block>
        </Box>
      </Row>
    </div>
  ),
};

/** Children side by side in the reader's direction, in their row's middle, a gap apart; those that do not fit wrap. */
export const RowWrapping: Story = {
  name: "Row",
  render: () => (
    <div style={frame}>
      <Row wrap align="center" gap="entry-caption">
        {["One", "Two, a longer one", "Three", "Four", "Five, the longest of them all", "Six"].map((name, i) => (
          <Block key={name} style={i % 2 === 1 ? { ...quiet, paddingBlock: rem(14) } : {}}>
            {name}
          </Block>
        ))}
      </Row>
    </div>
  ),
};

/** Inside a control or a line of text: a glyph and its label, as wide as what they hold. */
export const RowInline: Story = {
  name: "Row, inline",
  render: () => (
    <p style={{ ...type, margin: 0 }}>
      A line of text with{" "}
      <Row as="span" inline align="center" gap="chip-gap" className="rounded-control bg-surface-raised px-chip-inline">
        <span aria-hidden="true">●</span>
        <span>a glyph and its label</span>
      </Row>{" "}
      inside it.
    </p>
  ),
};

/** Children sharing a height: the head keeps its own, the body takes the rest and scrolls what does not fit. */
export const ColumnScrolling: Story = {
  name: "Column",
  render: () => (
    <div style={{ ...frame, blockSize: rem(200) }}>
      <Column full="block" gap="entry-line">
        <Box rigid>
          <Block style={quiet}>Head</Block>
        </Box>
        <Stack grow shrink="block" scroll="y" gap="entry-line" tabIndex={0} role="group" aria-label="Body">
          {Array.from({ length: 8 }, (_, i) => (
            <Block key={i}>Line {i + 1}</Block>
          ))}
        </Stack>
      </Column>
    </div>
  ),
};

/** Children one under another, each as wide as the stack and never wider: a word longer than the stack breaks. */
export const StackOfBlocks: Story = {
  name: "Stack",
  render: () => (
    <div style={frame}>
      <Stack gap="entry-line">
        <Block>One</Block>
        <Block style={{ ...quiet, overflowWrap: "anywhere" }}>
          Averyverylongwordwithnoplacetobreakthatwouldpushacolumnpastitscontainer
        </Block>
        <Block>Three</Block>
      </Stack>
    </div>
  ),
};

/** Columns of a template: the first takes the room, the last is as wide as what it holds. */
export const GridTrailing: Story = {
  name: "Grid",
  render: () => (
    <div style={frame}>
      <Grid columns="trail" gap="entry-line" align="center">
        <Block>{long}</Block>
        <Block style={quiet}>End</Block>
        <Block>Short</Block>
        <Block style={quiet}>A longer end</Block>
      </Grid>
    </div>
  ),
};

/** Children over one another in one cell: the later over the earlier, in the cell's middle. */
export const LayersCentered: Story = {
  name: "Layers",
  render: () => (
    <div style={frame}>
      <Layers center>
        <Block style={{ ...quiet, boxSizing: "border-box", inlineSize: "100%", blockSize: rem(96) }}>{""}</Block>
        <Block>Over</Block>
      </Layers>
    </div>
  ),
};

/** A box held against the edges of its anchor: on its top and end corner, and over its middle. */
export const PinOnAnchor: Story = {
  name: "Pin",
  render: () => (
    <div style={frame}>
      <Box anchor>
        <Block style={{ ...quiet, blockSize: rem(96) }}>Anchor</Block>
        <Pin top="media-badge" end="media-badge">
          <Block>Corner</Block>
        </Pin>
        <Pin center className="size-avatar-post">
          <Block style={{ boxSizing: "border-box", blockSize: "100%", padding: 0 }}>{""}</Block>
        </Pin>
      </Box>
    </div>
  ),
};

/** A bar that scrolls with its region until it meets the top, then stays over what scrolls under it. */
export const StickyBar: Story = {
  name: "Sticky",
  render: () => (
    <div style={{ ...frame, padding: 0, blockSize: rem(192) }}>
      <Box scroll="y" full="block" tabIndex={0} role="group" aria-label="Region">
        <Block>Before the bar</Block>
        <Sticky top="0" layer={1}>
          <Block style={quiet}>Bar</Block>
        </Sticky>
        <Stack gap="entry-line">
          {Array.from({ length: 10 }, (_, i) => (
            <Block key={i}>Line {i + 1}</Block>
          ))}
        </Stack>
      </Box>
    </div>
  ),
};

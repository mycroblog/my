// SPDX-License-Identifier: AGPL-3.0-only

// The generic layout parts (layout.tsx), one story each: the part placing plain blocks, which stand for whatever a
// part of the design system gives it, with the props that say how. The blocks' own look (a tone, a padding, a size)
// is the story's; the placement is the part's alone. Then each part that arranges children with what it was not
// written for, and no prop that asks for anything: a word of 200 letters with no place to break, fifty children, a
// text four times its size. None of them makes its page wider than the viewport.

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
/** A word of 200 letters with no place to break. */
const word = "Anunbrokenwordwithnoplacetobreak".repeat(7).slice(0, 200);
const many = Array.from({ length: 50 }, (_, i) => i + 1);
/** A text four times the size of the body's, as a reader who zooms to 400% sees it at the minimum width. */
const large: CSSProperties = { fontSize: `${text.body.size * 4}rem`, lineHeight: 1.2 };

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
        <Block style={quiet}>Averyverylongwordwithnoplacetobreakthatwouldpushacolumnpastitscontainer</Block>
        <Block>Three</Block>
      </Stack>
    </div>
  ),
};

/** Two columns alike, row after row: each cell as wide as its column, whatever it holds. */
export const GridOfTwo: Story = {
  name: "Grid",
  render: () => (
    <div style={frame}>
      <Grid columns={2} gap="entry-line" align="start">
        <Block>{long}</Block>
        <Block style={quiet}>Beside it</Block>
        <Block>Short</Block>
        <Block style={quiet}>Under it</Block>
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

/** A child with one long word gives way to the row's width and the word breaks inside it; the rigid one keeps its size. */
export const RowLongWord: Story = {
  name: "Row, a long word",
  render: () => (
    <div style={frame}>
      <Row align="start" gap="entry-caption">
        <Block>{word}</Block>
        <Box rigid>
          <Block style={quiet}>End</Block>
        </Box>
      </Row>
    </div>
  ),
};

/** A short label that keeps its words whole beside a text that wraps: the text gives way first. */
export const RowWords: Story = {
  name: "Row, a label's words kept whole",
  render: () => (
    <div style={frame}>
      <Row align="start" gap="entry-caption">
        <Block>{long}</Block>
        <Box words>
          <Block style={quiet}>A value</Block>
        </Box>
      </Row>
    </div>
  ),
};

/** More children than a line holds, in a row that may wrap: they go on as many lines as they need. */
export const RowManyChildren: Story = {
  name: "Row, many children, wrapping",
  render: () => (
    <div style={frame}>
      <Row wrap gap="chip-spacing">
        {many.map((n) => (
          <Block key={n}>{n}</Block>
        ))}
      </Row>
    </div>
  ),
};

/** More rigid children than a line holds, in a row that may scroll: they keep their size and the row scrolls. */
export const RowManyChildrenScrolling: Story = {
  name: "Row, many children, scrolling",
  render: () => (
    <div style={frame}>
      <Row scroll="x" gap="chip-spacing" tabIndex={0} role="group" aria-label="Numbers">
        {many.map((n) => (
          <Box key={n} rigid>
            <Block>{n}</Block>
          </Box>
        ))}
      </Row>
    </div>
  ),
};

/** A text four times its size: its words wrap, and one wider than the row breaks. */
export const RowLargeText: Story = {
  name: "Row, large text",
  render: () => (
    <div style={frame}>
      <Row align="start" gap="entry-caption">
        <Block style={large}>Large text beside a neighbour</Block>
        <Box rigid>
          <Block style={quiet}>End</Block>
        </Box>
      </Row>
    </div>
  ),
};

/** Inside a line of text, a row is no wider than the line: its long word breaks. */
export const RowInlineLongWord: Story = {
  name: "Row, inline, a long word",
  render: () => (
    <p style={{ ...type, margin: 0 }}>
      A line of text with{" "}
      <Row as="span" inline align="center" gap="chip-gap" className="rounded-control bg-surface-raised px-chip-inline">
        <span aria-hidden="true">●</span>
        <span>{word}</span>
      </Row>{" "}
      inside it.
    </p>
  ),
};

/** A child that stands at the start of its column is no wider than the column: its long word breaks. */
export const ColumnLongWord: Story = {
  name: "Column, a long word",
  render: () => (
    <div style={frame}>
      <Column align="start" gap="entry-line">
        <Block style={quiet}>Head</Block>
        <Block>{word}</Block>
      </Column>
    </div>
  ),
};

/** Fifty children one under another: the stack grows down, never sideways. */
export const StackManyChildren: Story = {
  name: "Stack, many children",
  render: () => (
    <div style={frame}>
      <Stack gap="entry-line">
        {many.map((n) => (
          <Block key={n}>Line {n}</Block>
        ))}
      </Stack>
    </div>
  ),
};

/** A text four times its size in a stack: it wraps at the stack's width. */
export const StackLargeText: Story = {
  name: "Stack, large text",
  render: () => (
    <div style={frame}>
      <Stack gap="entry-line">
        <Block style={large}>Large text in a stack</Block>
        <Block style={quiet}>Under it</Block>
      </Stack>
    </div>
  ),
};

/** A long word in one of two columns alike: the column keeps its width and the word breaks. */
export const GridLongWord: Story = {
  name: "Grid, a long word",
  render: () => (
    <div style={frame}>
      <Grid columns={2} gap="entry-line" align="start">
        <Block style={quiet}>{word}</Block>
        <Block>Beside it</Block>
      </Grid>
    </div>
  ),
};

/** Fifty children in two columns alike: each cell as wide as its column. */
export const GridManyChildren: Story = {
  name: "Grid, many children",
  render: () => (
    <div style={frame}>
      <Grid columns={2} gap="entry-line">
        {many.map((n) => (
          <Block key={n}>Cell {n}</Block>
        ))}
      </Grid>
    </div>
  ),
};

/** A long word in the middle of its cell: no wider than the cell, it breaks. */
export const LayersLongWord: Story = {
  name: "Layers, a long word",
  render: () => (
    <div style={frame}>
      <Layers center>
        <Block style={{ ...quiet, boxSizing: "border-box", inlineSize: "100%", blockSize: rem(96) }}>{""}</Block>
        <Block>{word}</Block>
      </Layers>
    </div>
  ),
};

/** A pinned box with a long word: no wider than what it is held against, its word breaks. */
export const PinLongWord: Story = {
  name: "Pin, a long word",
  render: () => (
    <div style={frame}>
      <Box anchor>
        <Block style={{ ...quiet, blockSize: rem(192) }}>Anchor</Block>
        <Pin top="0" end="0">
          <Block>{word}</Block>
        </Pin>
      </Box>
    </div>
  ),
};

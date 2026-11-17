// SPDX-License-Identifier: AGPL-3.0-only

// The links that lead from a line of facts: a chip with a count, a badge, and a quiet link to a list. The words are
// samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Badge, ChipLink } from "./chip";
import { Row } from "./layout";
import { QuietLink } from "./quiet-link";

const rem = (px: number) => `${px / rootPx}rem`;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Links", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** More chips than a row holds, one with a long word: they wrap onto further rows. */
export const Chips: Story = {
  render: () => (
    <Column>
      <Row as="ul" wrap plain gap="screen-name-gap">
        {["bread", "sourdough", "baking", "Donaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamten"].map(
          (name, i) => (
            <li key={name}>
              <ChipLink href={`/tags/${name}`} count={String(i * 7 + 3)}>
                #{name}
              </ChipLink>
            </li>
          ),
        )}
        <li>
          <ChipLink href="/tags/none">#without</ChipLink>
        </li>
      </Row>
    </Column>
  ),
};

/** Labels on the raised surface. */
export const Badges: Story = {
  render: () => (
    <Column>
      <Row wrap gap="screen-name-gap">
        <Badge>Follows you</Badge>
        <Badge>Administrator</Badge>
        <Badge>Bot</Badge>
      </Row>
    </Column>
  ),
};

/** A quiet link in the text's color and in the muted one. */
export const Quiet: Story = {
  render: () => (
    <Column>
      <Row wrap gap="screen-name-gap">
        <QuietLink href="/following">Following</QuietLink>
        <QuietLink href="/followers" tone="muted">
          Followers
        </QuietLink>
      </Row>
    </Column>
  ),
};

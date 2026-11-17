// SPDX-License-Identifier: AGPL-3.0-only

// The controls of a choice: the options open to one answer and to several, held while an answer is in flight, and a
// share as a bar. The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { ChoiceList, ShareBar } from "./choice";

const rem = (px: number) => `${px / rootPx}rem`;

const items = [
  { key: "a", label: "Sourdough" },
  { key: "b", label: "Rye, with caraway seeds, the way it is baked in the north of the country" },
  { key: "c", label: "Baguette" },
];

const none = () => undefined;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Choice", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** One answer: radios, the second chosen. */
export const One: Story = {
  render: () => (
    <Column>
      <ChoiceList label="Bread" multiple={false} items={items} chosen={[1]} onChange={none} />
    </Column>
  ),
};

/** Several answers: checkboxes, two chosen. */
export const Several: Story = {
  render: () => (
    <Column>
      <ChoiceList label="Bread" multiple items={items} chosen={[0, 2]} onChange={none} />
    </Column>
  ),
};

/** Held while an answer is in flight: nothing is pressed. */
export const Held: Story = {
  render: () => (
    <Column>
      <ChoiceList label="Bread" multiple={false} items={items} chosen={[0]} onChange={none} disabled />
    </Column>
  ),
};

/** A share of a whole, as a bar across what holds it. */
export const Share: Story = {
  render: () => (
    <Column>
      <ShareBar share={0.62} />
    </Column>
  ),
};

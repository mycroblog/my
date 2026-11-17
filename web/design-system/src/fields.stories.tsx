// SPDX-License-Identifier: AGPL-3.0-only

// The fields with no label of their own: a date and a time, and a box of a few lines that grows with its text, each
// named for assistive technology. The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { BareTextArea, DateTimeField } from "./form-page";

const rem = (px: number) => `${px / rootPx}rem`;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Fields", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** A date and a time within limits. */
export const DateTime: Story = {
  render: () => {
    const [value, setValue] = useState("2026-11-16T09:00");
    return (
      <Column>
        <DateTimeField
          label="Date and time"
          value={value}
          onChange={setValue}
          min="2026-11-16T00:00"
          max="2027-11-16T00:00"
        />
      </Column>
    );
  },
};

/** A date and a time that is refused: the line of the text's color. */
export const DateTimeInvalid: Story = {
  render: () => (
    <Column>
      <DateTimeField
        label="Date and time"
        value="2026-11-15T09:00"
        onChange={() => undefined}
        min="2026-11-16T00:00"
        invalid
      />
    </Column>
  ),
};

/** A box that grows with its text, which has a line longer than the box and a word with no place to break. */
export const TextArea: Story = {
  render: () => {
    const [value, setValue] = useState(
      "A note of a few lines.\nThe second line is long enough to wrap in a box as wide as a phone's column.\nDonaudampfschifffahrtselektrizitätenhauptbetriebswerkbauunterbeamtengesellschaft",
    );
    return (
      <Column>
        <BareTextArea label="Note" value={value} onChange={setValue} maxLength={2000} />
      </Column>
    );
  },
};

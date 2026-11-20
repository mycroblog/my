// SPDX-License-Identifier: AGPL-3.0-only

// The fields with no label of their own: a date and a time, a date alone, and a box of a few lines that grows with its text, each
// named for assistive technology; a label of the page for one, and the fields of a checked choice set in under it.
// The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { NumberField } from "./field";
import { BareTextArea, CheckField, DateTimeField, FieldLabel, NestedFields, TextField } from "./form-page";
import { Stack } from "./layout";

const rem = (px: number) => `${px / rootPx}rem`;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the check of every story shows it in dark too.
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

/** A date alone, of the calendar (a payment's date). */
export const DateOnly: Story = {
  render: () => {
    const [value, setValue] = useState("2026-11-16");
    return (
      <Column>
        <DateTimeField date label="Date" value={value} onChange={setValue} min="2026-01-01" />
      </Column>
    );
  },
};

/** An amount, at the field's end as a number is. */
export const Amount: Story = {
  render: () => (
    <Column>
      <TextField id="amount" name="amount" label="Monthly price" inputMode="decimal" figures defaultValue="1500" />
    </Column>
  ),
};

/** A number that is refused: marked invalid, as the field of its error is. */
export const NumberInvalid: Story = {
  render: () => (
    <Column>
      <NumberField label="Role position" defaultValue={120} min={0} max={999} invalid />
    </Column>
  ),
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

/** A date and a time under a label of the page, in a field's label style. */
export const Labelled: Story = {
  render: () => {
    const [value, setValue] = useState("2026-11-16T09:00");
    return (
      <Column>
        <Stack gap="form-page-field-gap">
          <FieldLabel htmlFor="starts">Starts</FieldLabel>
          <DateTimeField id="starts" label="Starts" value={value} onChange={setValue} />
        </Stack>
      </Column>
    );
  },
};

/** The fields of a checked choice, set in under it behind a line. */
export const Nested: Story = {
  render: () => (
    <Column>
      <Stack gap="form-page-field-gap">
        <CheckField id="in-person" name="in-person" value="yes" touch label="In person" defaultChecked />
        <NestedFields>
          <TextField id="place-name" name="place-name" label="Name" defaultValue="Riverside garden" />
          <TextField id="address" name="address" label="Address" multiline rows={2} />
        </NestedFields>
      </Stack>
    </Column>
  ),
};

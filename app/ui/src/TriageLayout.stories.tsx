// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { Section } from "./Section";
import { TriageLayout } from "./TriageLayout";

const meta = {
  title: "Layout/TriageLayout",
  component: TriageLayout,
  args: {
    summary: (
      <Section surface="card" title="Summary">
        Report on @bob: spam, filed 2 days ago.
      </Section>
    ),
    decision: (
      <Section surface="card" title="Resolution">
        <Button variant="primary">Take the action and resolve</Button>
      </Section>
    ),
    evidence: (
      <>
        <Section surface="card" title="Reported posts">
          The posts the report points at.
        </Section>
        <Section surface="card" title="Notes">
          The moderators&apos; notes.
        </Section>
      </>
    ),
  },
} satisfies Meta<typeof TriageLayout>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two columns in a wide container, the decision beside the evidence; one column, the decision first, when narrow. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("button", { name: "Take the action and resolve" })).toBeVisible();
  },
};

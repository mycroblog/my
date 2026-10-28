// SPDX-License-Identifier: AGPL-3.0-only
// Stack stories: the gaps of the roles, a divided list with and without a frame, list markers, and a stack that
// becomes a row in a wide container. The plays check the element each story renders.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button } from "./Button";
import { Heading } from "./Heading";
import { Stack } from "./Stack";
import { Hint, Text } from "./Text";

function Block({ children }: { children: string }) {
  return <div className="rounded-md bg-surface-sunken px-3 py-2">{children}</div>;
}

const meta = {
  title: "Layout/Stack",
  component: Stack,
  args: {
    gap: "3",
    children: (
      <>
        <Block>First</Block>
        <Block>Second</Block>
        <Block>Third</Block>
      </>
    ),
  },
} satisfies Meta<typeof Stack>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Lines inside a card: `gap="3"`. */
export const Default: Story = {};

/** Sections of a page are `gap="6"`, blocks inside a page `gap="4"`, controls in a column `gap="2"`. */
export const Gaps: Story = {
  render: () => (
    <Stack gap="6">
      {(["1", "2", "3", "4", "6", "8"] as const).map((gap) => (
        <Stack key={gap} as="section" aria-label={`gap ${gap}`} gap={gap}>
          <Block>{`gap ${gap}`}</Block>
          <Block>{`gap ${gap}`}</Block>
        </Stack>
      ))}
    </Stack>
  ),
};

/** `align="start"` keeps buttons and short text at their own width. */
export const AlignStart: Story = {
  render: () => (
    <Stack align="start" gap="2">
      <Hint>Shown 20 of 132 lines.</Hint>
      <Button size="sm">Show all</Button>
    </Stack>
  ),
};

/** A list whose rows are separated by rules and carry no gap (web-client/16-look-states-and-wording.md G3), as a `ul` in a rounded frame. */
export const DividedList: Story = {
  render: () => (
    <Stack as="ul" aria-label="Domains" divided border="lg">
      {["example.com", "a-very-long-subdomain-of-a-federated-server.example.social", "مثال.example"].map((domain) => (
        <Stack as="li" key={domain} gap="1" paddingX="4" paddingY="3">
          <Text dir="auto" weight="semibold" wrap="anywhere" align="matchParent">
            {domain}
          </Text>
          <Hint>Suspended since 17 Oct 2026</Hint>
        </Stack>
      ))}
    </Stack>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("list", { name: "Domains" })).toBeVisible();
    await expect(canvas.getAllByRole("listitem")).toHaveLength(3);
  },
};

/** The same list without a frame, its rows padded only in the block direction. */
export const DividedListUnframed: Story = {
  render: () => (
    <Stack as="ul" aria-label="Jobs" divided>
      {["deliver", "fetch_remote_account_with_a_very_long_job_type_name_that_has_no_spaces"].map((job) => (
        <Stack as="li" key={job} container paddingY="3" gap="2">
          <Heading level={3} mono wrap="anywhere">
            <span dir="ltr">{job}</span>
          </Heading>
          <Hint>Failed 3 times</Hint>
        </Stack>
      ))}
    </Stack>
  ),
};

/** Markers for a list of running text, in a muted region whose heading keeps the body color. */
export const Markers: Story = {
  render: () => (
    <Stack as="section" aria-labelledby="stack-rules" gap="2" tone="muted">
      <Heading level={3} id="stack-rules" tone="fg">
        How the totals are cut
      </Heading>
      <Stack as="ul" marker="disc" gap="1">
        <li>Dates are in the instance&apos;s time zone.</li>
        <li>Weeks start on Monday.</li>
        <li>
          A payment for a year is spread over its twelve months, so a long sentence like this one wraps onto the next
          line.
        </li>
      </Stack>
      <Stack as="ol" marker="decimal" gap="1">
        <li>Record the payment.</li>
        <li>Check the totals.</li>
      </Stack>
    </Stack>
  ),
};

/** A form of narrow fields: `as="form"` with `width="sm"` and the gap of fields. */
export const NarrowForm: Story = {
  render: () => (
    <Stack
      as="form"
      aria-label="Search"
      width="sm"
      gap="4"
      onSubmit={(event) => {
        event.preventDefault();
      }}
    >
      <Block>Field</Block>
      <Block>Field</Block>
    </Stack>
  ),
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("form", { name: "Search" })).toBeVisible();
  },
};

/** Stacked in a narrow container, a centered row from @md. */
export const RowAtMedium: Story = {
  render: () => (
    <Stack container>
      <Stack gap="3" rowAt="md">
        <Block>An avatar</Block>
        <Text wrap="anywhere">A line that sits beside it from a medium container width and under it below.</Text>
      </Stack>
    </Stack>
  ),
};

// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Button, ButtonLink } from "./Button";
import { Link } from "./Link";
import { Masthead } from "./Masthead";
import { PageHeading } from "./PageHeading";
import { Wordmark } from "./Wordmark";

const meta = {
  title: "Layout/Masthead",
  component: Masthead,
  args: {
    heading: <PageHeading display>my example</PageHeading>,
    mark: <Wordmark name="my" decorative size="sm" />,
    kicker: <span dir="ltr">my.example</span>,
    lede: <p>A small server for people who bake, ride, and write about it.</p>,
    facts: [
      { key: "members", content: "120 members" },
      { key: "since", content: "Since March 2026" },
      { key: "operators", content: <Link href="#operators">Run by Olive</Link> },
    ],
    actions: (
      <>
        <ButtonLink href="#join" variant="primary" size="lg">
          Join
        </ButtonLink>
        <Button variant="secondary" size="lg">
          Sign in
        </Button>
        <Link href="#elsewhere" variant="standalone">
          Have an account on another server? Follow from there
        </Link>
      </>
    ),
  },
} satisfies Meta<typeof Masthead>;

export default meta;
type Story = StoryObj<typeof meta>;

/** The landing page's masthead: the domain above the title at the display size, the summary, the facts and the actions. */
export const Default: Story = {
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 1, name: "my example" })).toBeVisible();
    await expect(canvas.getByRole("list")).toHaveTextContent("120 members");
  },
};

/** Without a summary or facts (a new server that hides its counts): the title and the actions only. */
export const Minimal: Story = {
  render: ({ heading, mark, kicker, actions }) => (
    <Masthead heading={heading} mark={mark} kicker={kicker} actions={actions} />
  ),
};

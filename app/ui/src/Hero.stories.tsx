// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, ButtonLink } from "./Button";
import { Hero } from "./Hero";
import { PageHeading } from "./PageHeading";
import { Hint, Text } from "./Text";

const meta = {
  title: "Layout/Hero",
  component: Hero,
  args: {
    eyebrow: "Welcome to my",
    heading: <PageHeading spark="✳">Welcome to my example</PageHeading>,
    children: <Text as="p">A small server for gardeners.</Text>,
    actions: (
      <>
        <ButtonLink href="/auth/sign_up" variant="primary" size="lg">
          Create an account
        </ButtonLink>
        <Button size="lg">Sign in</Button>
      </>
    ),
  },
} satisfies Meta<typeof Hero>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** Two posts beside the text from the panel's @3xl width, under it below. */
export const WithAside: Story = {
  args: {
    aside: (
      <>
        <Text as="p">A first post from the server.</Text>
        <Text as="p">A second post from the server.</Text>
      </>
    ),
  },
};

/** Registration needs approval: a note says so under the ways in. */
export const WithNote: Story = {
  args: { note: <Hint>Moderators review each new account before it can be used.</Hint> },
};

/** No lead and no registration: only signing in. */
export const Minimal: Story = {
  args: {
    children: undefined,
    actions: (
      <Button variant="primary" size="lg">
        Sign in
      </Button>
    ),
    note: <Hint>This server is not accepting new accounts.</Hint>,
  },
};

// SPDX-License-Identifier: AGPL-3.0-only
// The page after a user deactivated their account (/deactivated, ADR-0145 §3): signed out, with the date of the deletion.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";

const meta = {
  title: "Screens/Settings/Deactivated",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/deactivated?deletion_due_at=2026-12-03T09%3A00%3A00Z", signedIn: false } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The date of the deletion and how to come back. */
export const Normal: Story = {};

/** Without a date in the address, the page still tells how to come back. */
export const WithoutDate: Story = { parameters: { screen: { path: "/deactivated", signedIn: false } } };

/** At the narrowest width. */
export const Narrow: Story = narrow;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

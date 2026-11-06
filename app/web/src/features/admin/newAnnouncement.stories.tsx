// SPDX-License-Identifier: AGPL-3.0-only
// The page that writes an announcement (/admin/announcements/new; web-client/screens/admin-announcements-new.md) in
// the states of ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { inLocale, narrow } from "../../test/stories";
import { adminScreen, withoutPermission } from "./test/stories";

const meta = {
  title: "Screens/Admin/New announcement",
  ...adminScreen("/admin/announcements/new"),
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The empty form, shown for a week and published at once by default. */
export const Normal: Story = {};

/** A moderator without manage_announcements. */
export const MissingPermission: Story = withoutPermission;

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

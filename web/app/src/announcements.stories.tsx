// SPDX-License-Identifier: AGPL-3.0-only

// The server's announcements (ADR-0125 §3, §5) in the states every screen is shown in (app.samples.tsx): their list,
// the unread marked and one ended; and the page of one, unread (opening it marks it read), read, and ended.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { minimumWidth, rightToLeft, screen, screenMeta } from "./app.samples";

const at = "/announcements";

const meta = {
  title: "Announcements",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

export const List: Story = {};
export const Empty: Story = { parameters: screen({ at, state: "empty" }) };
export const Loading: Story = { parameters: screen({ at, state: "loading" }) };
export const Failed: Story = { parameters: screen({ at, state: "error" }) };
export const LongText: Story = { parameters: screen({ at, state: "long" }) };
/** An unread announcement's page: opening it marks it read. */
export const Unread: Story = { parameters: screen({ at: "announcement:maintenance" }) };
/** A read announcement's page. */
export const Read: Story = { parameters: screen({ at: "announcement:welcome" }) };
/** The page of an announcement whose period is over. */
export const Ended: Story = { parameters: screen({ at: "announcement:ended" }) };
/** An announcement's page that could not be loaded, with the link to the list. */
export const PageFailed: Story = { parameters: screen({ at: "announcement:maintenance", state: "error" }) };
export const RightToLeft: Story = { globals: rightToLeft };
export const MinimumWidth: Story = { globals: minimumWidth };

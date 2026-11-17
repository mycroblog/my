// SPDX-License-Identifier: AGPL-3.0-only

// The search field in each state of its reader (#3126): empty, with words and Clear at its end, and busy while
// something is looked up afar, with no Clear. The words are placeholders.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { fn } from "storybook/test";
import { SearchField } from "./search-field";

const meta = {
  title: "Search field states",
  args: { onChange: fn() },
  render: (args) => (
    <SearchField
      id="search"
      label="Search"
      hint="People by name or handle, and hashtags"
      clearLabel="Clear"
      defaultValue={args.defaultValue}
      busy={args.busy}
      onChange={args.onChange}
    />
  ),
} satisfies Meta<{ onChange: (value: string) => void; defaultValue?: string; busy?: boolean }>;
export default meta;

type Story = StoryObj<typeof meta>;

/** Nothing written yet: no Clear. */
export const Empty: Story = {};

/** Words written: Clear stands at the field's end. */
export const WithWords: Story = { args: { defaultValue: "sam" } };

/** While an account is looked up afar the field is busy, and no Clear stands at its end. */
export const BusyWhileLookingUp: Story = { args: { defaultValue: "@kiri@far.example", busy: true } };

/** Not busy, the same words show Clear and nothing busy. */
export const NotBusy: Story = { args: { defaultValue: "@kiri@far.example" } };

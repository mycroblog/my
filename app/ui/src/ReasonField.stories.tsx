// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ReasonField } from "./ReasonField";

// Stories may spell out English; apps pass a react-intl message instead (web-client/README.md A9).
const formatRemaining = (remaining: number) =>
  remaining < 0 ? `${String(-remaining)} characters too many` : `${String(remaining)} characters left`;

const meta = {
  title: "Forms/ReasonField",
  component: ReasonField,
  args: { label: "Reason", description: "Shown to the account's owner.", characterLimit: 5000, formatRemaining },
} satisfies Meta<typeof ReasonField>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Empty: Story = {};

export const Filled: Story = {
  args: { defaultValue: "Repeated unsolicited advertising in replies, after a warning on 3 October." },
};

export const Required: Story = { args: { isRequired: true, isInvalid: true, errorMessage: "Give a reason." } };

export const OverTheLimit: Story = { args: { characterLimit: 20, defaultValue: "Repeated unsolicited advertising." } };

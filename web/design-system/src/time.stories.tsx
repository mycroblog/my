// SPDX-License-Identifier: AGPL-3.0-only

// The name line of a post's header with its relative time (samples.tsx): the name at the start, the visibility glyph
// and the time together at the end. Default, driven by the Controls, a story per age a post may have, from just now to
// last year, in the reader's locale, and the widest times beside long names.

import { rootPx, text } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { ago, NameLine, visibilityLabel, type Visibility } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

const second = 1000;
const minute = 60 * second;
const hour = 60 * minute;
const day = 24 * hour;

// Last year is December 20 of the year before.
const lastYear = () => new Date(new Date().getFullYear() - 1, 11, 20, 18, 30);
const ages = {
  "just now": () => ago(2 * second),
  "45 seconds": () => ago(45 * second),
  "3 minutes": () => ago(3 * minute),
  "2 hours": () => ago(2 * hour),
  yesterday: () => ago(day + minute),
  "6 days": () => ago(6 * day),
  "3 weeks": () => ago(21 * day),
  "last year": lastYear,
} as const;
type Age = keyof typeof ages;

type Args = { name: string; visibility: Visibility; age: Age; locale: "en" | "ja" };

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Name line",
  tags: ["themes"],
  args: { name: "Alice Chen", visibility: "public", age: "3 minutes", locale: "en" },
  argTypes: {
    name: { control: "text" },
    visibility: { control: "select", options: Object.keys(visibilityLabel) },
    age: { control: "select", options: Object.keys(ages), description: "How long ago it was posted" },
    locale: { control: "select", options: ["en", "ja"], description: "The reader's locale, which the time reads in" },
  },
  render: ({ name, visibility, age, locale }) => (
    <div style={{ fontSize: `${text.body.size}rem`, maxWidth: rem(560) }}>
      <NameLine name={name} visibility={visibility} date={ages[age]()} locale={locale} />
    </div>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

export const Default: Story = {};
export const JustNow: Story = { args: { age: "just now" } };
export const Seconds: Story = { args: { age: "45 seconds" } };
export const Minutes: Story = { args: { age: "3 minutes" } };
export const Hours: Story = { args: { age: "2 hours" } };
export const Yesterday: Story = { args: { age: "yesterday" } };
export const Days: Story = { args: { age: "6 days" } };
export const Weeks: Story = { args: { age: "3 weeks" } };
export const LastYear: Story = { args: { age: "last year" } };
export const Japanese: Story = { args: { name: "佐藤 美香", age: "yesterday", locale: "ja" } };
export const LongName: Story = {
  args: { name: "The Open Source Weekly Newsletter Collective", visibility: "followers", age: "last year" },
};
export const LongNameJapanese: Story = {
  args: {
    name: "株式会社みんなのマイクロブログ広報チーム公式アカウント",
    visibility: "followers",
    age: "last year",
    locale: "ja",
  },
};

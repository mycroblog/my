// SPDX-License-Identifier: AGPL-3.0-only

// The plain-text editor without a caller's nodes: Default, driven by its props in the Controls, and a story per state:
// empty with its placeholder, several lines, over a limit (the last characters marked), read only, one line (the
// `line` look of the content warning), and Arabic text that reads right to left on its own.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { PlainTextEditor } from "./editor";

type Args = {
  value: string;
  over: number;
  readOnly: boolean;
  look: "composer" | "line";
};

const sample = {
  lines: "Walked along the river this morning.\nThe first frost turned every leaf on the path silver.",
  arabic: "مشيت على طول النهر هذا الصباح، وكان الصقيع الأول على كل ورقة.",
};

/** The editor holding its own value, as a caller does. */
function Editor({ value, over, readOnly, look }: Args) {
  const [text, setText] = useState(value);
  return (
    <PlainTextEditor
      namespace="story"
      value={text}
      onChange={setText}
      label="Text"
      placeholder="What's on your mind?"
      over={over}
      readOnly={readOnly}
      look={look}
      multiline={look === "composer"}
      roomy
    />
  );
}

// Tagged `themes`: the story tests run it in dark too.
const meta = {
  title: "Editor",
  tags: ["themes"],
  args: { value: sample.lines, over: 0, readOnly: false, look: "composer" },
  argTypes: {
    value: { control: "text", description: "The text it holds" },
    over: { control: "number", description: "How many characters at its end are over a limit, marked" },
    readOnly: { control: "boolean", description: "Not to be written in (while a post is sent)" },
    look: { control: "select", options: ["composer", "line"], description: "The composer's text, or one line" },
  },
  render: (args) => (
    <div style={{ maxInlineSize: "37.5rem" }}>
      <Editor key={JSON.stringify(args)} {...args} />
    </div>
  ),
} satisfies Meta<Args>;
export default meta;

type Story = StoryObj<typeof meta>;

/** Two lines, as the composer holds them. */
export const Default: Story = {};
export const Empty: Story = { args: { value: "" } };
export const OverLimit: Story = { args: { over: 12 } };
export const ReadOnly: Story = { args: { readOnly: true } };
export const OneLine: Story = { args: { value: "Spoilers for the season finale", look: "line" } };
export const Arabic: Story = { args: { value: sample.arabic } };

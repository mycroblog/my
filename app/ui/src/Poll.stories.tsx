// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Poll, PollChoices, PollHint, PollResults, PollSummary } from "./Poll";

const emojis = [
  {
    shortcode: "blobcat",
    url: "data:image/svg+xml;utf8,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 8 8'%3E%3Ccircle cx='4' cy='4' r='4' fill='%23f59e0b'/%3E%3C/svg%3E",
    static_url:
      "data:image/svg+xml;utf8,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 8 8'%3E%3Ccircle cx='4' cy='4' r='4' fill='%23f59e0b'/%3E%3C/svg%3E",
  },
];

const options = [
  { key: 0, title: "Tea", share: "52%", ratio: 0.52, ownChoice: true },
  { key: 1, title: "Coffee", share: "38%", ratio: 0.38 },
  { key: 2, title: "Water :blobcat:", share: "10%", ratio: 0.1 },
];

const meta = {
  title: "Display/Poll",
  component: PollResults,
  args: { label: "Results", options, ownChoiceLabel: "Your vote" },
  decorators: [
    (Story) => (
      <div style={{ maxInlineSize: "24rem" }}>
        <Story />
      </div>
    ),
  ],
} satisfies Meta<typeof PollResults>;

export default meta;
type Story = StoryObj<typeof meta>;

const ends = { dateTime: "2026-10-23T09:00:00Z", absolute: "23 Oct 2026, 09:00", text: "Ends in 1 day" };

/** The results of an open poll, with the viewer's vote marked in words as well as by color. */
export const Results: Story = {
  render: (args) => (
    <Poll label="Poll">
      <PollResults {...args} emojis={emojis} />
      <PollSummary total="42 votes" ends={ends} />
    </Poll>
  ),
};

export const Closed: Story = {
  render: (args) => (
    <Poll label="Poll">
      <PollResults {...args} />
      <PollSummary total="42 votes" closedLabel="Closed" remoteLabel="Counts reported by remote.example" />
    </Poll>
  ),
};

/** The author hides the votes of each option until the poll ends. */
export const HiddenResults: Story = {
  args: { options: options.map(({ key, title }) => ({ key, title })) },
  render: (args) => (
    <Poll label="Poll">
      <PollResults {...args} />
      <PollHint>The votes are shown when the poll ends.</PollHint>
      <PollSummary total="42 votes" ends={ends} />
    </Poll>
  ),
};

export const LongOptions: Story = {
  args: {
    options: [
      {
        key: 0,
        title: "Supercalifragilisticexpialidocious_and_then_some_more_words_without_any_break_at_all",
        share: "70%",
        ratio: 0.7,
      },
      {
        key: 1,
        title: "A rather long option that has to wrap onto several lines in a narrow column of the timeline",
        share: "30%",
        ratio: 0.3,
        ownChoice: true,
      },
    ],
  },
};

export const Rtl: Story = {
  args: {
    options: [
      { key: 0, title: "شاي", share: "60%", ratio: 0.6, ownChoice: true },
      { key: 1, title: "قهوة", share: "40%", ratio: 0.4 },
    ],
    ownChoiceLabel: "صوتك",
  },
  decorators: [
    (Story) => (
      <div dir="rtl">
        <Story />
      </div>
    ),
  ],
};

function Choices({ multiple }: { multiple: boolean }) {
  const [selected, setSelected] = useState<string[]>([]);
  return (
    <Poll label="Poll">
      <PollChoices
        label="Options"
        multiple={multiple}
        description={multiple ? "Choose one or more" : "Choose one"}
        options={options.map((option) => ({ value: String(option.key), title: option.title }))}
        emojis={emojis}
        selected={selected}
        onChange={(value, on) => {
          setSelected((current) =>
            multiple ? (on ? [...current, value] : current.filter((v) => v !== value)) : [value],
          );
        }}
        submitLabel="Vote"
        canSubmit={selected.length > 0}
        onSubmit={() => undefined}
      />
      <PollSummary total="42 votes" ends={ends} />
    </Poll>
  );
}

export const SingleChoiceForm: Story = { render: () => <Choices multiple={false} /> };

export const MultipleChoiceForm: Story = { render: () => <Choices multiple /> };

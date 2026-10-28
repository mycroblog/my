// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { AttachmentItem } from "./AttachmentItem";
import { Box } from "./Box";
import { Button } from "./Button";
import { InlineError } from "./Text";
import { Stack } from "./Stack";
import { TextArea } from "./TextArea";

const image =
  "data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 160 90'%3E%3Crect width='160' height='90' fill='%2314b8a6'/%3E%3C/svg%3E";

const meta = {
  title: "Compose/AttachmentItem",
  component: AttachmentItem,
  args: {
    src: image,
    children: (
      <>
        <TextArea
          label="Description of image 1"
          description="Describe the image for people who cannot see it."
          rows={2}
        />
        <Box self="start">
          <Button size="sm" variant="ghost">
            Remove image 1
          </Button>
        </Box>
      </>
    ),
  },
  render: (args) => (
    <Box container width="prose">
      <Stack as="ul" aria-label="Images" gap="3">
        <AttachmentItem {...args} />
      </Stack>
    </Box>
  ),
} satisfies Meta<typeof AttachmentItem>;

export default meta;
type Story = StoryObj<typeof meta>;

/** Uploaded, with its description field and the remove button. */
export const Ready: Story = {};

/** The upload runs: a spinner band over the preview. */
export const Uploading: Story = { args: { progressLabel: "Uploading" } };

/** The preview is not there yet (processing on the server). */
export const Processing: Story = { args: { src: null, progressLabel: "Processing" } };

/** The upload failed: the error above the description field. */
export const Failed: Story = {
  args: {
    children: (
      <>
        <InlineError>Could not upload: the file is too large.</InlineError>
        <TextArea label="Description of image 1" rows={2} />
      </>
    ),
  },
};

/** The description is long and has a long word. */
export const WithDescription: Story = {
  args: {
    children: (
      <TextArea
        label="Description of image 1"
        defaultValue="Supercalifragilisticexpialidocious_Supercalifragilisticexpialidocious_Supercalifragilistic"
        rows={2}
      />
    ),
  },
};

/** In a narrow container the preview sits above the fields. */
export const Narrow: Story = {
  decorators: [
    (Story) => (
      <div style={{ width: "20rem" }}>
        <Story />
      </div>
    ),
  ],
};

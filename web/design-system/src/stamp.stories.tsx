// SPDX-License-Identifier: AGPL-3.0-only

// The stamp of an entry: the time alone, led to a page, and led to a page by a target as tall as a row of avatars.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "./avatar";
import { Stamp } from "./entry";
import { Row } from "./layout";
import { ago, images } from "./samples";

// Tagged `themes`: the check of every story shows it in dark too.
const meta = { title: "Stamp", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The time, a link to the entry. */
export const Linked: Story = { render: () => <Stamp date={ago(5 * 60_000)} href="/@mika/1" /> };

/** The link reaches the height of the avatar beside it. */
export const Tall: Story = {
  render: () => (
    <Row align="center" gap="entry-caption">
      <Avatar name="Mika" src={images.dusk} size="row" decorative />
      <Stamp date={ago(5 * 60_000)} href="/@mika/1" tall />
    </Row>
  ),
};

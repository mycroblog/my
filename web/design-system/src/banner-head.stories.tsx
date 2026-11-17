// SPDX-License-Identifier: AGPL-3.0-only

// The head of a page with a picture: the banner and the lead over its lower edge with what the reader may do at the
// other end of its row, with and without a banner. In a column as narrow as a phone's the banner reaches the edges;
// in a wider one it is a card of its own. The words are samples in English.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Avatar } from "./avatar";
import { BannerHead } from "./banner-head";
import { Button } from "./button";
import { images } from "./samples";

const rem = (px: number) => `${px / rootPx}rem`;

function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), paddingInline: rem(16) }}>{children}</div>;
}

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Banner head", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const lead = <Avatar name="Alice Chen" src={images.dusk} size="preview" decorative />;
const end = (
  <Button kind="secondary" size="timeline">
    Follow
  </Button>
);

/** A banner picture, the lead over its edge and an action at the other end. */
export const WithBanner: Story = {
  render: () => (
    <Column>
      <BannerHead banner={images.dusk} lead={lead} end={end} labelledBy="banner-name" dir="ltr">
        <h2 id="banner-name">Alice Chen</h2>
        <p>The rest of the head, a gap under the row.</p>
      </BannerHead>
    </Column>
  ),
};

/** No banner: the raised surface stands in, with no action. */
export const WithoutBanner: Story = {
  render: () => (
    <Column>
      <BannerHead lead={lead} labelledBy="banner-name" dir="ltr">
        <h2 id="banner-name">Alice Chen</h2>
      </BannerHead>
    </Column>
  ),
};

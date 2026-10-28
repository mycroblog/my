// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, waitFor } from "storybook/test";
import { Button } from "./Button";
import { CustomEmoji } from "./CustomEmoji";
import { ExternalLink } from "./ExternalLink";
import { Heading } from "./Heading";
import { Link } from "./Link";
import { ProfileField, ProfileFields, ProfileHeader, ProfileHeaderImage } from "./ProfileHeader";
import { Wordmark } from "./Wordmark";

/** A stand-in image, inline so the story does not depend on the network. */
function svgImage(width: number, height: number, fill: string): { still: { url: string; width: number }[] } {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 ${String(width)} ${String(height)}"><rect width="${String(width)}" height="${String(height)}" fill="${fill}"/><circle cx="${String(width / 2)}" cy="${String(height / 2)}" r="${String(height / 3)}" fill="#f4d7b5"/></svg>`;
  return { still: [{ url: `data:image/svg+xml,${encodeURIComponent(svg)}`, width }] };
}
const header = svgImage(1500, 500, "#3b6fd8");
const avatar = svgImage(128, 128, "#d8703b");
/** Bytes no browser can decode, so the image fails to load at once. */
const broken = { still: [{ url: "data:image/png;base64,AAAA" }] };

const heart = {
  shortcode: "blobheart",
  url: svgImage(32, 32, "#d83b6f").still[0]?.url ?? "",
  static_url: svgImage(32, 32, "#d83b6f").still[0]?.url ?? "",
};

const fields = (
  <ProfileFields>
    <ProfileField term="Website">
      <ExternalLink href="https://example.com">example.com</ExternalLink>
    </ProfileField>
    <ProfileField term="Pronouns">they/them</ProfileField>
  </ProfileFields>
);

const meta = {
  title: "Display/ProfileHeader",
  component: ProfileHeader,
  args: {
    image: header,
    avatar,
    name: "Alice Example",
    heading: (
      <Heading level={1}>
        <span dir="auto">Alice Example</span>
      </Heading>
    ),
    handle: "@alice@example.social",
    actions: <Button variant="primary">Follow</Button>,
    tags: ["Follows you", "Bot"],
    note: <p>Writes about gardens and trains.</p>,
    fields,
    joined: "Joined 3 Mar 2024",
    counts: [
      {
        key: "following",
        content: (
          <Link href="#following" variant="standalone">
            120 following
          </Link>
        ),
      },
      {
        key: "followers",
        content: (
          <Link href="#followers" variant="standalone">
            3,400 followers
          </Link>
        ),
      },
    ],
  },
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "40rem" }}>
        <Story />
      </div>
    ),
  ],
  play: async ({ canvas }) => {
    await expect(canvas.getByRole("heading", { level: 1 })).toBeVisible();
  },
} satisfies Meta<typeof ProfileHeader>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {};

/** No header image: a plain band of the same height. */
export const WithoutHeaderImage: Story = { args: { image: undefined, avatar: undefined } };

/** An image that fails to load falls back to the band. */
export const BrokenHeaderImage: Story = {
  args: { image: broken },
  play: async ({ canvasElement }) => {
    await waitFor(() => {
      if (canvasElement.querySelector("img[src^='data:image/png']") !== null)
        throw new Error("the broken image is still shown");
    });
  },
};

/** A long name and a long handle wrap instead of scrolling the page. */
export const LongNames: Story = {
  args: {
    name: "Bartholomew Maximilian Featherstonehaugh-Cholmondeley",
    heading: (
      <Heading level={1}>
        <span dir="auto">Bartholomew Maximilian Featherstonehaugh-Cholmondeley the Third of Westminster</span>
      </Heading>
    ),
    handle: "@bartholomew_maximilian_featherstonehaugh_cholmondeley@a-very-long-subdomain.example.social",
  },
};

/** The page reads right to left; the handle still reads left to right. */
export const RightToLeft: Story = {
  args: {
    name: "أليس",
    heading: (
      <Heading level={1}>
        <span dir="auto">أليس</span>
      </Heading>
    ),
    note: <p dir="auto">أكتب عن الحدائق والقطارات.</p>,
    fields: (
      <ProfileFields>
        <ProfileField term="الموقع">example.com</ProfileField>
        <ProfileField term="الضمائر">هي</ProfileField>
      </ProfileFields>
    ),
    joined: "انضمت في 3 مارس 2024",
  },
  decorators: [
    (Story) => (
      <div dir="rtl">
        <Story />
      </div>
    ),
  ],
};

/** Custom emoji in the name and the field names. */
export const CustomEmojiInName: Story = {
  args: {
    heading: (
      <Heading level={1}>
        <span dir="auto">
          Alice <CustomEmoji emoji={heart} />
        </span>
      </Heading>
    ),
    fields: (
      <ProfileFields>
        <ProfileField term={<CustomEmoji emoji={heart} />}>Alice</ProfileField>
      </ProfileFields>
    ),
  },
};

/** Many fields with long values. */
export const ManyFields: Story = {
  args: {
    fields: (
      <ProfileFields>
        {Array.from({ length: 6 }, (_, i) => (
          <ProfileField key={i} term={`Field number ${String(i + 1)}`}>
            https://a-very-long-address.example.com/with/a/path/that/goes/on/and/on/{i}
          </ProfileField>
        ))}
      </ProfileFields>
    ),
  },
};

/** A narrow container: the fields stack their names above their values. */
export const Narrow: Story = {
  decorators: [
    (Story) => (
      <div style={{ maxWidth: "20rem" }}>
        <Story />
      </div>
    ),
  ],
};

/** A server's calling card on its landing page: its mark in the avatar's place, its domain as the handle, no joined line. */
export const Server: Story = {
  args: {
    avatar: undefined,
    mark: <Wordmark name="my" decorative size="sm" />,
    name: "Garden Club",
    heading: (
      <Heading level={1}>
        <span dir="auto">Garden Club</span>
      </Heading>
    ),
    handle: "garden.example",
    actions: <Button variant="secondary">Sign in</Button>,
    tags: [],
    note: <p>A small server for gardeners and everyone who likes to grow things.</p>,
    fields: undefined,
    joined: undefined,
    counts: [
      { key: "members", content: "120 members" },
      { key: "since", content: "Since March 2026" },
    ],
  },
};

/** The 3:1 preview of a form. */
export const Preview: StoryObj<typeof ProfileHeaderImage> = {
  render: () => <ProfileHeaderImage sources={header} size="preview" />,
  play: async ({ canvasElement }) => {
    await expect(canvasElement.querySelector("img")).toBeVisible();
  },
};

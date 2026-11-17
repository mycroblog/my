// SPDX-License-Identifier: AGPL-3.0-only

// The faces of several people in a row: one, a few, and more than `limit` shows. The names are samples.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Avatar } from "./avatar";
import { Faces } from "./faces";
import { images } from "./samples";

const people = [
  { name: "Mika", src: images.dusk },
  { name: "Ren", src: images.snow },
  { name: "Sora", src: images.logo },
  { name: "Aoi Tanaka", src: undefined },
  { name: "Haruto", src: images.dusk },
];

const faces = (count: number) =>
  people.slice(0, count).map((p) => <Avatar key={p.name} name={p.name} src={p.src} size="row" decorative />);

// Tagged `themes`: the story tests run it in dark too.
const meta = { title: "Faces", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** One person. */
export const One: Story = { render: () => <Faces>{faces(1)}</Faces> };

/** Three people, each a little over the next. */
export const Three: Story = { render: () => <Faces>{faces(3)}</Faces> };

/** Five people, of whom the first three show. */
export const Limited: Story = { render: () => <Faces limit={3}>{faces(5)}</Faces> };

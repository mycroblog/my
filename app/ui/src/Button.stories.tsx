// SPDX-License-Identifier: AGPL-3.0-only
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { spacing } from "./storySpacing";
import { Button, ButtonLink } from "./Button";
import { CloseIcon, MoreIcon, PlusIcon, ReplyIcon, StarIcon } from "./Icons";
import { ToggleButton } from "./ToggleButton";

const meta = {
  title: "Forms/Button",
  component: Button,
  args: { children: "Post" },
} satisfies Meta<typeof Button>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Primary: Story = { args: { variant: "primary" } };
export const Secondary: Story = { args: { variant: "secondary" } };
export const Outline: Story = { args: { variant: "outline", children: "Follow" } };
export const Ghost: Story = { args: { variant: "ghost" } };
/** `size="lg"`: the calls to action of a hero. */
export const Large: Story = { args: { variant: "primary", size: "lg", children: "Create an account" } };

/** A link that looks like a button: it opens another page. */
export const AsLink: Story = {
  render: () => (
    <ButtonLink href="/auth/sign_up" variant="primary" size="lg">
      Create an account
    </ButtonLink>
  ),
};
export const Danger: Story = { args: { variant: "danger", children: "Delete post" } };

const variants = ["primary", "secondary", "outline", "ghost", "danger"] as const;
const sizes = ["sm", "md"] as const;

/** Every variant × size, enabled, disabled and pending. */
export const Matrix: Story = {
  render: () => (
    <div className="flex flex-col gap-4">
      {sizes.map((size) => (
        <div key={size} className="flex flex-wrap items-center gap-3">
          {variants.map((variant) => (
            <Button key={variant} variant={variant} size={size}>
              {variant} {size}
            </Button>
          ))}
          {variants.map((variant) => (
            <Button key={`${variant}-disabled`} variant={variant} size={size} isDisabled>
              {variant} disabled
            </Button>
          ))}
          <Button variant="primary" size={size} isPending>
            Posting…
          </Button>
        </div>
      ))}
    </div>
  ),
};

/** A long label wraps instead of overflowing a narrow container. */
export const LongLabel: Story = {
  args: { variant: "primary", children: "Publish this post to everyone who follows you, and to the local timeline" },
};

/** An icon-only button is named with aria-label. */
export const IconOnly: Story = {
  args: { variant: "ghost", "aria-label": "Close", children: <CloseIcon /> },
};

/**
 * The button sizes the icon in it, which has no size of its own: `size-5`, and `size-4` in a small
 * button. The icon keeps its size when a long label wraps.
 */
export const WithIcon: Story = {
  render: () => (
    <div className="flex flex-col items-start gap-3">
      {sizes.map((size) => (
        <div key={size} className="flex flex-wrap items-center gap-3">
          <Button size={size} variant="primary">
            <PlusIcon />
            New post {size}
          </Button>
          <Button size={size} variant="ghost" aria-label={`Close ${size}`}>
            <CloseIcon />
          </Button>
        </div>
      ))}
      <Button className="max-w-48">
        <PlusIcon />
        Publish this post to everyone who follows you
      </Button>
    </div>
  ),
  play: async ({ canvas }) => {
    const iconOf = (name: string) => canvas.getByRole("button", { name }).querySelector("svg")?.getBoundingClientRect();
    for (const [name, steps] of [
      ["New post md", 5],
      ["Close md", 5],
      ["New post sm", 4],
      ["Close sm", 4],
      ["Publish this post to everyone who follows you", 5],
    ] as const) {
      const icon = iconOf(name);
      await expect(icon?.width, name).toBeCloseTo(spacing(steps), 1);
      await expect(icon?.height, name).toBeCloseTo(spacing(steps), 1);
    }
  },
};

/**
 * In a row of actions next to `ToggleButton`s (a post's), a `toolbar` button is as tall as they are and
 * sizes its icon as they do (`size-5`).
 */
export const InToolbar: Story = {
  render: () => (
    <div className="flex flex-wrap items-center gap-1 text-fg-muted">
      <Button variant="ghost" size="toolbar" aria-label="Reply, 3" className="text-fg-muted">
        <ReplyIcon />
        <span className="tabular-nums">3</span>
      </Button>
      <ToggleButton aria-label="Favourite">
        <StarIcon />
      </ToggleButton>
      <Button variant="ghost" size="toolbar" aria-label="More" className="text-fg-muted">
        <MoreIcon />
      </Button>
    </div>
  ),
  play: async ({ canvas }) => {
    const boxOf = (name: string) => canvas.getByRole("button", { name }).getBoundingClientRect();
    const iconOf = (name: string) => canvas.getByRole("button", { name }).querySelector("svg")?.getBoundingClientRect();
    for (const name of ["Reply, 3", "Favourite", "More"]) {
      await expect(iconOf(name)?.width, name).toBeCloseTo(spacing(5), 1);
      await expect(iconOf(name)?.height, name).toBeCloseTo(spacing(5), 1);
      await expect(boxOf(name).height, name).toBe(boxOf("Favourite").height);
    }
  },
};

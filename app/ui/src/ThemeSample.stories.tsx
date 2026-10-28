// SPDX-License-Identifier: AGPL-3.0-only
// Representative components under the sample server theme (ADR-0059, .storybook/sampleTheme.ts). The story
// fixes the `instanceTheme` global, so its screenshots, taken in every story project like any other story's,
// show the overrides of colors, spacing and radius, while the other stories keep the default tokens.
// The toolbar applies the same theme to any story by hand.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect } from "storybook/test";
import { Badge } from "./Badge";
import { Button } from "./Button";
import { Checkbox } from "./Checkbox";
import { PlusIcon, ReplyIcon, StarIcon } from "./Icons";
import { Link } from "./Link";
import { Switch } from "./Switch";
import { TextField } from "./TextField";
import { ToggleButton } from "./ToggleButton";

export default {
  title: "Foundations/Theme sample",
  globals: { instanceTheme: "sample" },
} satisfies Meta;

type Story = StoryObj;

function Gallery() {
  return (
    <div className="flex flex-col gap-6">
      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">Actions</h2>
        <div className="flex flex-wrap items-center gap-2">
          <Button variant="primary">
            <PlusIcon />
            New post
          </Button>
          <Button>Cancel</Button>
          <Button variant="ghost">Skip</Button>
          <Button variant="danger">Delete</Button>
          <Button size="sm">
            <ReplyIcon />
            Reply
          </Button>
          <ToggleButton aria-label="Favourite">
            <StarIcon />
          </ToggleButton>
        </div>
      </section>
      <section className="flex flex-col gap-4 rounded-lg bg-surface-raised px-4 py-3 shadow-md">
        <h2 className="text-lg font-semibold">Profile</h2>
        <TextField
          label="Display name"
          description="Shown on your profile and next to your posts."
          defaultValue="Alice Liddell"
        />
        <Checkbox defaultSelected>Require approval for new followers</Checkbox>
        <Switch defaultSelected>Show sensitive media</Switch>
      </section>
      <section className="flex flex-col gap-3">
        <h2 className="text-lg font-semibold">Text</h2>
        <p>
          Read the{" "}
          <Link href="#rules" variant="inline">
            server rules
          </Link>{" "}
          before you post.
        </p>
        <p className="text-sm text-fg-muted">Posted 2 hours ago</p>
        <div className="flex flex-wrap items-center gap-2">
          <Badge count={3} label="3 unread" />
          <Badge count={12} label="12 reports" tone="danger" />
          <Badge count={1} label="1 draft" tone="neutral" />
        </div>
      </section>
    </div>
  );
}

export const Components: Story = {
  render: () => <Gallery />,
  play: async () => {
    // The overrides apply: the spacing step of the sample theme, not the default 0.25rem.
    await expect(getComputedStyle(document.documentElement).getPropertyValue("--spacing").trim()).toBe("0.3rem");
  },
};

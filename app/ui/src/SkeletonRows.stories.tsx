// SPDX-License-Identifier: AGPL-3.0-only
// Skeleton row presets in a loading region each: the region is aria-busy and has one loading label, and the rows stay
// out of the accessibility tree. Every story is also captured with prefers-reduced-motion: reduce, where the setup fails
// on any running animation (.storybook/vitest.setup.ts); the plays check the pulse without it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ComponentType } from "react";
import { expect } from "storybook/test";
import {
  AccountRowSkeleton,
  FactsCardSkeleton,
  NotificationRowSkeleton,
  PostSkeleton,
  type SkeletonRowProps,
  StageSkeleton,
  VoiceRoomCardSkeleton,
} from "./SkeletonRows";
import { VisuallyHidden } from "./VisuallyHidden";

const meta = {
  title: "Display/SkeletonRows",
  component: PostSkeleton,
} satisfies Meta<typeof PostSkeleton>;

export default meta;
type Story = StoryObj<typeof meta>;

const keys = ["first", "second", "third"] as const;

type LoadingListProps = {
  title: string;
  label: string;
  Row: ComponentType<SkeletonRowProps>;
  /** How the rows sit: divided by borders (account lists), apart (cards), or flush (posts, notifications). */
  layout?: "divided" | "gapped" | "flush";
};

const layouts = {
  divided: "flex flex-col divide-y divide-border border-y border-border",
  gapped: "flex flex-col gap-3",
  flush: "flex flex-col",
} as const;

function LoadingList({ title, label, Row, layout = "flush" }: LoadingListProps) {
  const id = `skeleton-rows-${title.toLowerCase()}`;
  return (
    <section aria-labelledby={id} aria-busy="true" className="flex flex-col gap-2">
      <h2 id={id} className="px-4 text-lg font-semibold">
        {title}
      </h2>
      <VisuallyHidden>{label}</VisuallyHidden>
      <div className={layouts[layout]}>
        {keys.map((key) => (
          <Row key={key} />
        ))}
      </div>
    </section>
  );
}

/** The region is busy and named once; every shape is hidden, and pulses only with no reduced-motion preference. */
async function expectLoadingRegion(canvasElement: HTMLElement, name: string, label: string) {
  const region = canvasElement.querySelector<HTMLElement>("section[aria-busy='true']");
  await expect(region).not.toBeNull();
  await expect(region?.textContent).toBe(`${name}${label}`);
  const reduce = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const rows = canvasElement.querySelectorAll(":scope section > div > [aria-hidden='true']");
  await expect(rows).toHaveLength(keys.length);
  for (const shape of canvasElement.querySelectorAll(".bg-border")) {
    await expect(shape.closest("[aria-hidden='true']")).not.toBeNull();
    await expect(getComputedStyle(shape).animationName).toBe(reduce ? "none" : "pulse");
  }
}

export const Posts: Story = {
  render: () => <LoadingList title="Posts" label="Loading posts" Row={PostSkeleton} />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "Posts" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "Posts", "Loading posts");
  },
};

export const Accounts: Story = {
  render: () => <LoadingList title="Followers" label="Loading followers" Row={AccountRowSkeleton} layout="divided" />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "Followers" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "Followers", "Loading followers");
  },
};

export const Notifications: Story = {
  render: () => <LoadingList title="Notifications" label="Loading notifications" Row={NotificationRowSkeleton} />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "Notifications" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "Notifications", "Loading notifications");
  },
};

export const VoiceRooms: Story = {
  render: () => <LoadingList title="Rooms" label="Loading rooms" Row={VoiceRoomCardSkeleton} layout="gapped" />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "Rooms" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "Rooms", "Loading rooms");
  },
};

export const FactsCards: Story = {
  render: () => <LoadingList title="History" label="Loading history" Row={FactsCardSkeleton} layout="gapped" />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "History" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "History", "Loading history");
  },
};

export const Stage: Story = {
  render: () => <LoadingList title="Room" label="Loading the room" Row={StageSkeleton} />,
  play: async ({ canvas, canvasElement }) => {
    await expect(canvas.getByRole("region", { name: "Room" })).toHaveAttribute("aria-busy", "true");
    await expectLoadingRegion(canvasElement, "Room", "Loading the room");
  },
};

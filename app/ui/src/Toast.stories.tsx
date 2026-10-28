// SPDX-License-Identifier: AGPL-3.0-only
// Toast stories. The play functions test that showing a toast announces it without moving focus, that
// dismissing a focused toast hands focus on, and that auto-dismiss waits while the pointer is over it.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, fn, waitFor } from "storybook/test";
import { Button } from "./Button";
import { AnnouncerProvider } from "./Announcer";
import { ToastProvider, useToast } from "./Toast";

function Demo({
  onUndo,
  timeout,
  celebrate = false,
}: {
  onUndo: () => void;
  timeout?: number | null | undefined;
  celebrate?: boolean;
}) {
  const toast = useToast();
  const timing = timeout === undefined ? {} : { timeout };
  return (
    <div className="flex flex-wrap gap-2">
      <Button
        onPress={() => {
          toast.show({ message: "Draft saved", tone: "success", celebrate, ...timing });
        }}
      >
        Save draft
      </Button>
      <Button
        onPress={() => {
          toast.show({ message: "Post deleted", action: { label: "Undo", onAction: onUndo }, ...timing });
        }}
      >
        Delete post
      </Button>
      <Button
        onPress={() => {
          toast.show({ message: "Could not send the post", tone: "danger", ...timing });
        }}
      >
        Send
      </Button>
    </div>
  );
}

type DemoArgs = {
  onUndo: () => void;
  timeout?: number | null | undefined;
  defaultTimeout?: number;
  celebrate?: boolean;
};

const meta = {
  title: "Feedback/Toast",
  // The toasts are fixed to the bottom of the story frame.
  decorators: [
    (Story) => (
      <div className="min-h-80">
        <Story />
      </div>
    ),
  ],
  args: { onUndo: fn(), timeout: null },
  render: ({ defaultTimeout, ...args }) => (
    <AnnouncerProvider>
      <ToastProvider
        regionLabel="Notifications"
        dismissLabel="Dismiss"
        {...(defaultTimeout === undefined ? {} : { defaultTimeout })}
      >
        <Demo {...args} />
      </ToastProvider>
    </AnnouncerProvider>
  ),
} satisfies Meta<DemoArgs>;

export default meta;
type Story = StoryObj<typeof meta>;

export const AnnouncesWithoutFocus: Story = {
  play: async ({ canvas, userEvent }) => {
    const save = canvas.getByRole("button", { name: "Save draft" });
    await userEvent.click(save);
    const region = await canvas.findByRole("region", { name: "Notifications" });
    await expect(region).toHaveTextContent("Draft saved");
    await expect(canvas.getByRole("status")).toHaveTextContent("Draft saved");
    await expect(save).toHaveFocus();

    const send = canvas.getByRole("button", { name: "Send" });
    await userEvent.click(send);
    await expect(canvas.getByRole("alert")).toHaveTextContent("Could not send the post");
    await expect(send).toHaveFocus();
  },
};

/** Three tones and an action, stacked. */
export const Stack: Story = {
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Save draft" }));
    await userEvent.click(canvas.getByRole("button", { name: "Delete post" }));
    await userEvent.click(canvas.getByRole("button", { name: "Send" }));
    await expect(await canvas.findAllByRole("listitem")).toHaveLength(3);
    // Leave no hover or focus state in the screenshot.
    await userEvent.unhover(canvas.getByRole("button", { name: "Send" }));
    canvas.getByRole("button", { name: "Send" }).blur();
  },
};

/** The action runs and closes its toast; closing the focused last toast returns focus to where it came from. */
export const ActionAndDismiss: Story = {
  play: async ({ args, canvas, userEvent }) => {
    const save = canvas.getByRole("button", { name: "Save draft" });
    await userEvent.click(save);
    await userEvent.click(canvas.getByRole("button", { name: "Delete post" }));
    await userEvent.click(await canvas.findByRole("button", { name: "Undo" }));
    await expect(args.onUndo).toHaveBeenCalledOnce();
    await waitFor(() => expect(canvas.getAllByRole("listitem")).toHaveLength(1));

    await userEvent.click(save);
    const dismissButtons = canvas.getAllByRole("button", { name: "Dismiss" });
    await expect(dismissButtons).toHaveLength(2);
    // Keyboard users reach the toasts after the page content, here from Send.
    save.focus();
    await userEvent.tab();
    await userEvent.tab();
    await userEvent.tab();
    await expect(dismissButtons[0]).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(canvas.getAllByRole("listitem")).toHaveLength(1));
    await expect(canvas.getByRole("button", { name: "Dismiss" })).toHaveFocus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(canvas.queryByRole("region", { name: "Notifications" })).not.toBeInTheDocument());
    await expect(canvas.getByRole("button", { name: "Send" })).toHaveFocus();
  },
};

/** A toast without an action closes by itself, but not while the pointer rests on it. */
export const AutoDismissPausesOnHover: Story = {
  args: { timeout: undefined, defaultTimeout: 300 },
  play: async ({ canvas, userEvent }) => {
    await userEvent.click(canvas.getByRole("button", { name: "Save draft" }));
    const toast = await canvas.findByRole("listitem");
    await userEvent.hover(toast);
    await new Promise((resolve) => setTimeout(resolve, 600));
    await expect(toast).toBeInTheDocument();
    await userEvent.unhover(toast);
    await waitFor(() => expect(canvas.queryByRole("listitem")).not.toBeInTheDocument());
  },
};

/**
 * A toast of a celebrated moment (web-client §21.5, such as the first post): its icon grows in with the overshoot of
 * `ease-celebrate` as it appears (the `soft` fill instead with reduced motion), for a moment only. The other toasts stay
 * quiet.
 */
export const Celebrate: Story = {
  tags: ["no-screenshot"],
  args: { celebrate: true },
  play: async ({ canvas, userEvent, step }) => {
    await step("the toast celebrates as it appears", async () => {
      await userEvent.click(canvas.getByRole("button", { name: "Save draft" }));
      const toast = await canvas.findByRole("listitem");
      const icon = toast.querySelector("[data-celebrating]");
      await expect(icon).not.toBeNull();
      const animations = icon?.querySelector("svg")?.getAnimations() ?? [];
      // Under reduced motion nothing moves; the moment shows by the fill alone.
      if (!window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
        await expect(animations.map((a) => (a instanceof CSSAnimation ? a.animationName : ""))).toContain("celebrate");
      } else {
        await expect(animations).toHaveLength(0);
      }
    });
    await step("the moment ends by itself, and the toast stays", async () => {
      const toast = canvas.getByRole("listitem");
      await waitFor(() => expect(toast.querySelector("[data-celebrating]")).toBeNull(), { timeout: 2000 });
      await expect(toast).toHaveTextContent("Draft saved");
    });
    await step("another toast does not celebrate", async () => {
      await userEvent.click(canvas.getByRole("button", { name: "Send" }));
      await waitFor(() => expect(canvas.getAllByRole("listitem")).toHaveLength(2));
      await expect(canvas.getAllByRole("listitem")[1]?.querySelector("[data-celebrating]")).toBeNull();
    });
  },
};

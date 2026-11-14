// SPDX-License-Identifier: AGPL-3.0-only

// The plays of the Shell stories that check what only a rendered shell shows, in the story's locale: the call's bar
// stays across the screens and its controls answer, and no update toast shows during a call.

import { catalogReader } from "@my/design-system";
import type { StoryContext } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { screenMeta } from "./app.samples";
import { messages, type MessageId } from "./messages.gen";

const stringsIn = catalogReader<MessageId>(messages);

/** The elements of `elements` that show at the story's width (the shell keeps a part per width, one showing). */
const shown = (elements: HTMLElement[]) => elements.filter((e) => e.getClientRects().length > 0);

/** The one of `elements` that shows, or a failure that names `what`. */
function showing(elements: HTMLElement[], what: string): HTMLElement {
  const [element] = shown(elements);
  if (element === undefined) throw new Error(`the shell shows no ${what}`);
  return element;
}

/**
 * In a call with a newer build waiting: no toast offers the update; Mute mutes and unmutes; the bar stays on the
 * notifications and back home.
 */
export async function callAcrossScreens(context: StoryContext): Promise<void> {
  await screenMeta.play(context);
  const t = stringsIn(String(context.globals["locale"] ?? "en")).t;
  const canvas = within(context.canvasElement);
  const mute = () => showing(canvas.getAllByRole("button", { name: t("app.call_mute") }), "Mute");
  await expect(canvas.queryByRole("button", { name: t("app.update") })).toBeNull();

  await userEvent.click(mute());
  await waitFor(() => expect(mute()).toHaveAttribute("aria-pressed", "true"));
  await userEvent.click(mute());
  await waitFor(() => expect(mute()).toHaveAttribute("aria-pressed", "false"));

  const notifications = showing(
    canvas.getAllByRole("link", { name: (name) => name.startsWith(t("app.notifications")) }),
    "Notifications",
  );
  await userEvent.click(notifications);
  await waitFor(() => expect(notifications).toHaveAttribute("aria-current", "page"));
  await expect(mute()).toBeVisible();

  const home = showing(canvas.getAllByRole("link", { name: t("app.home") }), "Home");
  await userEvent.click(home);
  await waitFor(() => expect(home).toHaveAttribute("aria-current", "page"));
  await screenMeta.play(context);
  await expect(mute()).toBeVisible();
}

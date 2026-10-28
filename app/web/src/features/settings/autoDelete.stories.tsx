// SPDX-License-Identifier: AGPL-3.0-only
// The automatic deletion of old posts (/settings/auto-delete; web-client/screens/settings-auto-delete.md) in the states
// of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, waitFor, within } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import {
  answering,
  failingOn,
  inLocale,
  loadingOn,
  narrow,
  renderScreen,
  screenLoader,
  settled,
} from "../../test/stories";
import { autoDeletePolicyFixture, autoDeletePreviewOf } from "./test/handlers";

/** The server's policy of the viewer, which PUT replaces. */
function autoDeletePolicy(initial: Schemas["AutoDeletePolicy"]) {
  let policy = initial;
  return [
    http.get(`${apiBase}/preferences/auto_delete`, ({ request }) =>
      withViewer(request, () => HttpResponse.json(policy)),
    ),
    http.put(`${apiBase}/preferences/auto_delete`, async ({ request }) => {
      const saved = (await request.clone().json()) as Schemas["AutoDeletePolicy"];
      return withViewer(request, () => {
        policy = saved;
        return HttpResponse.json(policy);
      });
    }),
  ];
}

const meta = {
  title: "Screens/Settings/AutoDelete",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/settings/auto-delete",
      signedIn: true,
      handlers: autoDeletePolicy({ ...autoDeletePolicyFixture, enabled: true, min_age_days: 90, keep_min_boosts: 5 }),
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The live region of the estimate, with the count of the posts it would delete. */
function previewCount(canvasElement: HTMLElement): HTMLElement {
  const section = within(canvasElement).getByRole("region", { name: "Posts these conditions delete" });
  return within(section).getByRole("status");
}

/**
 * Posts older than 90 days are deleted; keeping media too counts one post fewer before the save, and saving says what
 * will be deleted.
 */
export const Normal: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await waitFor(async () => {
      await expect(previewCount(context.canvasElement)).toHaveTextContent(/Posts to delete now\s*2$/);
    });
    await userEvent.click(canvas.getByRole("checkbox", { name: "With images or other media" }));
    await waitFor(async () => {
      await expect(previewCount(context.canvasElement)).toHaveTextContent(/Posts to delete now\s*1$/);
    });
    await userEvent.click(canvas.getByRole("button", { name: "Save" }));
    await expect(
      await canvas.findByText("Saved. Posts older than 90 days will be deleted, except those you keep."),
    ).toBeInTheDocument();
  },
};

/** The server's default policy: nothing is deleted. */
export const Empty: Story = answering(...autoDeletePolicy(autoDeletePolicyFixture));

/** The policy has not arrived yet. */
export const Loading: Story = loadingOn("get", "/preferences/auto_delete");

/** The server failed to answer the policy. */
export const ServerError: Story = failingOn("get", "/preferences/auto_delete");

/** The posts are still being counted. */
export const PreviewLoading: Story = loadingOn("post", "/preferences/auto_delete/preview");

/** The server failed to count the posts: the form stays, and the estimate says why. */
export const PreviewError: Story = {
  ...failingOn("post", "/preferences/auto_delete/preview"),
  play: async (context) => {
    await settled(context);
    await expect(
      await context.canvas.findByText(/^The posts could not be counted: /, {}, { timeout: 10_000 }),
    ).toBeVisible();
  },
};

/** More posts than the server counts: the count stops at its limit and says there are more. */
export const PreviewMany: Story = answering(
  http.post(`${apiBase}/preferences/auto_delete/preview`, async ({ request }) => {
    const policy = (await request.clone().json()) as Schemas["AutoDeletePolicy"];
    return withViewer(request, () => HttpResponse.json({ ...autoDeletePreviewOf(policy), count: 1000, more: true }));
  }),
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

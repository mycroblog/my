// SPDX-License-Identifier: AGPL-3.0-only
// The article editor (/articles/new; web-client/screens/article-editor.md) in the states of ADR-0079 §13: a draft
// being written with its preview, a new article, the preview on its way, a draft that failed to load, a conflict
// with another device's save, Japanese, and narrow.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, userEvent, within } from "storybook/test";
import { draftFixture, type Schemas } from "../../test/fixtures";
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

const text = [
  "Two pots, one stubborn plant, and a long list of what the soil asked for.",
  "Use a **deep pot**, and mix compost into the soil before the first planting of the season.",
].join("\n\n");

const draft: Schemas["Draft"] = {
  ...draftFixture("d1", { text, language: "en" }),
  article: {
    title: "What I learned growing tomatoes on a balcony",
    summary: "",
    tags: ["garden", "tomatoes"],
    images: [],
  },
};

/** GET /drafts/d1 answering the draft with its ETag. */
const draftHandler = http.get(`${apiBase}/drafts/d1`, ({ request }) =>
  withViewer(request, () => HttpResponse.json(draft, { headers: { ETag: '"v1"' } })),
);

const meta = {
  title: "Screens/Article editor",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/articles/new?draft=d1", signedIn: true, handlers: [draftHandler] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A draft being written, beside the server's rendering of its body. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    const canvas = within(context.canvasElement);
    const preview = await canvas.findByRole("region", { name: "Preview" });
    await expect(await within(preview).findByText("deep pot", { selector: "strong" })).toBeVisible();
  },
};

/** A new article: nothing written yet. */
export const Empty: Story = { parameters: { screen: { path: "/articles/new" } } };

/** The draft has not arrived yet. */
export const Loading: Story = loadingOn("get", "/drafts/d1");

/** The server failed to answer the draft. */
export const ServerError: Story = failingOn("get", "/drafts/d1");

/** Another device saved the draft since it was opened: the text stays, and it can be saved as a new draft. */
export const Conflict: Story = {
  ...answering(
    draftHandler,
    http.put(`${apiBase}/drafts/d1`, () =>
      HttpResponse.json({ type: "about:blank", title: "Precondition Failed", status: 412 }, { status: 412 }),
    ),
  ),
  play: async (context) => {
    await settled(context);
    const canvas = within(context.canvasElement);
    await userEvent.type(canvas.getByRole("textbox", { name: "Body" }), " More soon.");
    await expect(
      await canvas.findByText(/^This draft was changed on another device/, {}, { timeout: 5000 }),
    ).toBeVisible();
  },
};

/** The editor in Japanese. */
export const Japanese: Story = inLocale("ja");

/** The editor on a phone: the preview under the form. */
export const Narrow: Story = { ...narrow };

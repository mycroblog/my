// SPDX-License-Identifier: AGPL-3.0-only
// The keyword filters (/settings/filters; web-client/screens/settings-filters.md) in the states of ADR-0079 §13.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
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
import { keywordFilterFixture } from "./test/handlers";

/** The server's filters of the viewer, oldest first. */
function keywordFilters(items: Schemas["KeywordFilter"][]) {
  return [
    http.get(`${apiBase}/filters`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ items } satisfies Schemas["KeywordFilterCollection"])),
    ),
  ];
}

const examples: Schemas["KeywordFilter"][] = [
  keywordFilterFixture("f1", {
    title: "Season finale",
    action: "warn",
    contexts: ["home", "notifications", "public", "thread", "account"],
    keywords: [
      { keyword: "finale", whole_word: true },
      { keyword: "ネタバレ", whole_word: false },
      { keyword: "spoiler", whole_word: true },
    ],
  }),
  keywordFilterFixture("f2", {
    title: "Election night",
    action: "hide",
    contexts: ["public"],
    keywords: ["vote", "poll", "ballot", "turnout", "count", "results", "exit poll"].map((keyword) => ({
      keyword,
      whole_word: false,
    })),
    expires_at: "2027-11-05T09:00:00Z",
  }),
  {
    ...keywordFilterFixture("f3", {
      title: "Conference",
      action: "warn",
      contexts: ["home"],
      keywords: [{ keyword: "#conf2026", whole_word: false }],
    }),
    expires_at: "2026-10-01T09:00:00Z",
    expired: true,
  },
];

const meta = {
  title: "Screens/Settings/KeywordFilters",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: "/settings/filters", signedIn: true, handlers: keywordFilters(examples) },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Three filters: one that folds, one that hides until a date, and one that expired. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("Expired")).toBeInTheDocument();
  },
};

/**
 * The form that adds a filter, open in place of the button with focus in its name. (The unit tests submit it: the
 * submit button leaves with the panel, and React Aria's announcement of its pending state would name a gone element.)
 */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Add a filter" }));
    const field = canvas.getByRole("textbox", { name: /Name/ });
    await expect(field).toHaveFocus();
    await userEvent.type(field, "Season finale");
    await userEvent.type(canvas.getByRole("textbox", { name: "Word 1" }), "finale");
    await expect(canvas.getByRole("link", { name: /How words match/ })).toBeInTheDocument();
  },
};

/** The form that changes a filter, in its row, with the choice that keeps its end. */
export const Editing: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Edit: ⁨Election night⁩" }));
    await expect(canvas.getByRole("textbox", { name: /Name/ })).toHaveValue("Election night");
  },
};

/** No filter: the empty state leads to adding one. */
export const Empty: Story = {
  ...answering(...keywordFilters([])),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("button", { name: "Add a filter" })).toBeInTheDocument();
  },
};

/** The filters have not arrived yet. */
export const Loading: Story = loadingOn("get", "/filters");

/** The server failed to answer the filters. */
export const ServerError: Story = failingOn("get", "/filters");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

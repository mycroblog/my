// SPDX-License-Identifier: AGPL-3.0-only
// The CSV exports and imports (/settings/import-export) in the states of
// ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
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
import { dataImports } from "./test/storyFixtures";

/** GET /imports answering `items`. */
function importing(items: Schemas["DataImport"][]) {
  return http.get(`${apiBase}/imports`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const meta = {
  title: "Screens/Settings/ImportExport",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/import-export", signedIn: true, handlers: [importing(dataImports)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** An import in progress, one finished with failures, and one finished cleanly. */
export const Normal: Story = {};

/** No imports yet. */
export const Empty: Story = answering(importing([]));

/** The imports have not arrived yet. */
export const Loading: Story = loadingOn("get", "/imports");

/** The server failed to answer the imports. */
export const ServerError: Story = failingOn("get", "/imports");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

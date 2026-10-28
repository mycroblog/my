// SPDX-License-Identifier: AGPL-3.0-only
// The aliases (/settings/aliases; web-client/screens/settings-aliases.md) in the states of ADR-0112 §1.
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

type Alias = Schemas["AccountAlias"];

const old: Alias = {
  uri: "https://old.example/users/alice",
  acct: "alice@old.example",
  created_at: "2026-10-01T09:00:00Z",
};

/** The server's aliases of the viewer, oldest first, which adding and removing change. */
function aliases(initial: Alias[]) {
  let items = initial;
  return [
    http.get(`${apiBase}/accounts/aliases`, ({ request }) =>
      withViewer(request, () => HttpResponse.json({ aliases: items })),
    ),
    http.post(`${apiBase}/accounts/aliases`, async ({ request }) => {
      const { acct } = (await request.clone().json()) as { acct: string };
      return withViewer(request, () => {
        const name = acct.replace(/^@/, "");
        const [user, host] = name.split("@");
        items = [
          ...items,
          { uri: `https://${host ?? ""}/users/${user ?? ""}`, acct: name, created_at: "2026-10-29T09:00:00Z" },
        ];
        return HttpResponse.json({ aliases: items });
      });
    }),
    http.post(`${apiBase}/accounts/aliases/remove`, async ({ request }) => {
      const { uri } = (await request.clone().json()) as { uri: string };
      return withViewer(request, () => {
        items = items.filter((alias) => alias.uri !== uri);
        return HttpResponse.json({ aliases: items });
      });
    }),
  ];
}

const meta = {
  title: "Screens/Settings/Aliases",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/settings/aliases", signedIn: true, handlers: aliases([old]) } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** One alias, with the count beside the heading and the button that adds another. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("1 alias")).toBeInTheDocument();
  },
};

/**
 * The form that adds an alias, open in place of the button with focus in its field. (The unit tests submit it: the
 * submit button leaves with the panel, and React Aria's announcement of its pending state would name a gone element.)
 */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Add an alias" }));
    const field = canvas.getByRole("textbox", { name: /Account to move from/ });
    await expect(field).toHaveFocus();
    await userEvent.type(field, "bob@other.example");
  },
};

/** At the most aliases: a note stands in place of the button. */
export const Full: Story = answering(
  ...aliases(
    ["a", "b", "c", "d", "e"].map((name, index) => ({
      uri: `https://x.example/users/${name}`,
      acct: `${name}@x.example`,
      created_at: `2026-10-0${String(index + 1)}T00:00:00Z`,
    })),
  ),
);

/** No alias: the empty state leads to adding one. */
export const Empty: Story = answering(...aliases([]));

/** The aliases have not arrived yet. */
export const Loading: Story = loadingOn("get", "/accounts/aliases");

/** The server failed to answer the aliases. */
export const ServerError: Story = failingOn("get", "/accounts/aliases");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

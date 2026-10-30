// SPDX-License-Identifier: AGPL-3.0-only
// The lists page (/lists; web-client/screens/lists.md, ADR-0122) in the states of ADR-0112 §1, with a list's members
// opened in a play.
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { within } from "storybook/test";
import type { I18n } from "../../lib/i18n";
import { otherAccountFixture, remoteAccountFixture, type Schemas } from "../../test/fixtures";
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
import { friendsList, listFixture } from "./test/handlers";

/** GET /lists answering `items`. */
function lists(items: Schemas["List"][]) {
  return http.get(`${apiBase}/lists`, ({ request }) => withViewer(request, () => HttpResponse.json({ items })));
}

const someLists = [
  friendsList,
  listFixture("0192f0c4-7a8e-7000-8000-00000000a002", "Bakers and bread people", { members_count: 12 }),
  // A name in a right-to-left script (web-client/README.md Y10).
  listFixture("0192f0c4-7a8e-7000-8000-00000000a003", "أصدقاء", { members_count: 3 }),
];

const meta = {
  title: "Screens/Lists/Lists",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: { screen: { path: "/lists", signedIn: true, handlers: [lists(someLists)] } },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

type Context = StoryContext & { canvasElement: HTMLElement };

/** The message `key` of the story's locale, without arguments. */
function message(context: Context, key: "client:lists.members.manage"): string {
  const { i18n } = context.loaded as { i18n: I18n };
  return i18n.messages[key];
}

/** Three lists, one with a right-to-left name. */
export const Normal: Story = {};

/** No list yet: the empty state holds the button that creates one. */
export const Empty: Story = answering(lists([]));

/** The most lists: a note stands in place of the button. */
export const Full: Story = answering(
  lists(
    Array.from({ length: 50 }, (_, i) =>
      listFixture(`0192f0c4-7a8e-7000-8000-00000000b${String(i).padStart(3, "0")}`, `List ${String(i + 1)}`),
    ),
  ),
);

/** The members of the first list, opened, with the search of the accounts the user follows. */
export const Members: Story = {
  parameters: {
    screen: {
      path: "/lists",
      signedIn: true,
      handlers: [
        lists(someLists),
        http.get(`${apiBase}/lists/:id/accounts`, () =>
          HttpResponse.json({ accounts: [otherAccountFixture, remoteAccountFixture] }),
        ),
      ],
    },
  },
  play: async (context) => {
    await settled(context);
    const [manage] = await context.canvas.findAllByRole("button", {
      name: new RegExp(message(context, "client:lists.members.manage")),
    });
    if (manage === undefined) throw new Error("no list to manage");
    await context.userEvent.click(manage);
    await within(context.canvasElement).findAllByRole("link", { name: /Bob|Carol/ });
  },
};

/** The lists have not arrived yet. */
export const Loading: Story = loadingOn("get", "/lists");

/** The server failed to answer. */
export const ServerError: Story = failingOn("get", "/lists");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

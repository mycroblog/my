// SPDX-License-Identifier: AGPL-3.0-only
// The "Add to list" dialog of a followed account's profile (web-client/screens/profile.md §4, ADR-0122) in the states
// of ADR-0112 §1. Each story opens the dialog from the profile's menu in its play.
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { within } from "storybook/test";
import type { I18n } from "../../lib/i18n";
import { relationshipFixture } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { friendsList, listFixture } from "./test/handlers";

type Context = StoryContext & { canvasElement: HTMLElement };

/** The message `key` of the story's locale, without arguments. */
function message(context: Context, key: "client:lists.add_to_list.open"): string {
  const { i18n } = context.loaded as { i18n: I18n };
  return i18n.messages[key];
}

/** Opens the dialog from the profile's menu, in any locale: the menu button is the header's last button. */
async function openDialog(context: Context) {
  await settled(context);
  const header = await context.canvas.findByRole("banner").catch(() => context.canvasElement);
  const buttons = within(header).getAllByRole("button");
  const more = buttons.find((button) => button.getAttribute("aria-haspopup") === "true");
  if (more === undefined) throw new Error("the profile has no menu");
  await context.userEvent.click(more);
  const body = within(document.body);
  await context.userEvent.click(
    await body.findByRole("menuitem", { name: message(context, "client:lists.add_to_list.open") }),
  );
  await body.findByRole("dialog");
}

/** The viewer follows Bob. */
const following = http.get(`${apiBase}/accounts/relationships`, ({ request }) =>
  withViewer(request, () =>
    HttpResponse.json(
      new URL(request.url).searchParams.getAll("id").map((id) => relationshipFixture(id, { following: true })),
    ),
  ),
);

const meta = {
  title: "Screens/Lists/AddToList",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/@bob",
      signedIn: true,
      handlers: [
        following,
        http.get(`${apiBase}/lists`, () =>
          HttpResponse.json({
            items: [
              friendsList,
              listFixture("0192f0c4-7a8e-7000-8000-00000000a002", "Bakers"),
              listFixture("0192f0c4-7a8e-7000-8000-00000000a003", "Full list", { members_count: 1000 }),
            ],
          }),
        ),
      ],
    },
  },
  play: openDialog,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Bob is in Friends; the full list cannot be checked. */
export const Normal: Story = {};

/** No list yet: the dialog leads to the lists page. */
export const NoLists: Story = answering(
  following,
  http.get(`${apiBase}/lists`, () => HttpResponse.json({ items: [] })),
);

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

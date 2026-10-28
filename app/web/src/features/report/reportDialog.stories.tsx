// SPDX-License-Identifier: AGPL-3.0-only
// The report dialog (features/report; web-client/screens/moderation.md), opened from a post's menu and from a
// profile's menu, at each of its steps and in the states of ADR-0112 §1. Each story opens the dialog in its play.
import type { Meta, StoryContext, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect, waitFor, within } from "storybook/test";
import type { I18n } from "../../lib/i18n";
import { type Schemas, statusFixture } from "../../test/fixtures";
import { apiBase, problem } from "../../test/msw/http";
import { answering, inLocale, narrow, renderScreen, screenLoader, settled } from "../../test/stories";
import { instanceWithRules } from "./test/handlers";

const postId = "0192f0c4-7a8e-7000-8000-0000000000c1";
const post: Schemas["Status"] = {
  ...statusFixture(postId),
  content: "<p>Buy the best sunglasses at half price, today only! Click the link in my profile.</p>",
};

/** Bob's post the dialog starts from, with an empty thread. */
const servePost = [
  http.get(`${apiBase}/statuses/:id/context`, () => HttpResponse.json({ ancestors: [], descendants: [] })),
  http.get(`${apiBase}/statuses/:id`, ({ params }) =>
    params["id"] === postId ? HttpResponse.json(post) : problem(404, "/api/v1/problems/not-found", "Not Found"),
  ),
];

type Context = StoryContext & { canvasElement: HTMLElement };

/** The message `key` of the story's locale, without arguments. */
function message(context: Context, key: "client:status.action.more" | "client:report.open"): string {
  const { i18n } = context.loaded as { i18n: I18n };
  return i18n.messages[key];
}

/** Opens the report dialog from the menu of the post the page is about, in any locale. */
async function openFromPost(context: Context): Promise<HTMLElement> {
  const { canvas, userEvent } = context;
  await settled(context);
  const [more] = await canvas.findAllByRole("button", { name: message(context, "client:status.action.more") });
  if (more === undefined) throw new Error("the post has no menu");
  await userEvent.click(more);
  const body = within(document.body);
  await userEvent.click(await body.findByRole("menuitem", { name: message(context, "client:report.open") }));
  return body.findByRole("dialog");
}

/** Waits until the dialog's step `name` is shown with the focus on its heading. */
async function expectStep(dialog: HTMLElement, name: string) {
  const heading = await within(dialog).findByRole("heading", { name });
  await waitFor(() => expect(heading).toHaveFocus());
}

/** Opens the dialog, chooses `reason` and goes on to the next step. */
async function chooseReason(context: Context, reason: string): Promise<HTMLElement> {
  const dialog = await openFromPost(context);
  await expectStep(dialog, "Why are you reporting this account?");
  await context.userEvent.click(within(dialog).getByRole("radio", { name: reason }));
  await context.userEvent.click(within(dialog).getByRole("button", { name: "Next" }));
  return dialog;
}

/** Goes from the posts step to the last one, with the posts as they are chosen. */
async function toComment(context: Context, dialog: HTMLElement) {
  await expectStep(dialog, "Which posts are you reporting?");
  await within(dialog).findAllByRole("checkbox");
  await context.userEvent.click(within(dialog).getByRole("button", { name: "Next" }));
  await expectStep(dialog, "Anything else the moderators should know?");
}

const meta = {
  title: "Screens/Moderation/ReportDialog",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: { path: `/@bob/status/${postId}`, signedIn: true, handlers: [instanceWithRules, ...servePost] },
  },
  play: async (context) => {
    await openFromPost(context);
  },
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** The first step, opened from a post's menu: why the account is reported. */
export const Normal: Story = {
  play: async (context) => {
    const dialog = await openFromPost(context);
    await expectStep(dialog, "Why are you reporting this account?");
  },
};

/** A rule violation asks which of the server's rules it breaks. */
export const Rules: Story = {
  play: async (context) => {
    const dialog = await chooseReason(context, "Breaks the server rules");
    await expectStep(dialog, "Which rules does it break?");
  },
};

/** The posts to report: the post the dialog started from, chosen, and the account's latest posts. */
export const Posts: Story = {
  play: async (context) => {
    const dialog = await chooseReason(context, "Spam");
    await expectStep(dialog, "Which posts are you reporting?");
    await expect(await within(dialog).findAllByRole("checkbox")).not.toHaveLength(0);
  },
};

/** The last step: a comment for the moderators. */
export const Comment: Story = {
  play: async (context) => {
    const dialog = await chooseReason(context, "Something else");
    await toComment(context, dialog);
  },
};

/** The report was sent. */
export const Sent: Story = {
  play: async (context) => {
    const dialog = await chooseReason(context, "Spam");
    await toComment(context, dialog);
    await context.userEvent.click(within(dialog).getByRole("button", { name: "Send report" }));
    await expectStep(dialog, "Report sent");
  },
};

/** The server refused the report: the dialog says why and keeps the answers. */
export const Refused: Story = {
  ...answering(
    http.post(`${apiBase}/reports`, () =>
      problem(429, "/api/v1/problems/rate-limited", "Too many reports in the last hour."),
    ),
  ),
  play: async (context) => {
    const dialog = await chooseReason(context, "Spam");
    await toComment(context, dialog);
    await context.userEvent.type(
      within(dialog).getByRole("textbox", { name: "Comment" }),
      "Sells things in every reply",
    );
    await context.userEvent.click(within(dialog).getByRole("button", { name: "Send report" }));
    await expect(await within(dialog).findByRole("alert")).toBeInTheDocument();
  },
};

/** Opened from the profile of another server's account: the last step offers to forward the report. */
export const FromRemoteProfile: Story = {
  parameters: { screen: { path: "/@carol@remote.example" } },
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(await canvas.findByRole("button", { name: "More actions for ⁨Carol⁩" }));
    const body = within(document.body);
    await userEvent.click(await body.findByRole("menuitem", { name: "Report" }));
    const dialog = await body.findByRole("dialog", { name: "Report ⁨Carol⁩" });
    await expectStep(dialog, "Why are you reporting this account?");
    await userEvent.click(within(dialog).getByRole("radio", { name: "Illegal content" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    await toComment(context, dialog);
    await expect(within(dialog).getByRole("checkbox", { name: /remote\.example/ })).toBeInTheDocument();
  },
};

/** In Japanese. */
export const Japanese: Story = {
  ...inLocale("ja"),
  play: async (context) => {
    await inLocale("ja").play(context);
    await openFromPost(context);
  },
};

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = {
  ...inLocale("ar-XB"),
  play: async (context) => {
    await inLocale("ar-XB").play(context);
    await openFromPost(context);
  },
};

/** At 320 CSS px. */
export const Narrow: Story = narrow;

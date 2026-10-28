// SPDX-License-Identifier: AGPL-3.0-only
// The muted domains (/settings/domain-mutes; web-client/screens/settings-domain-mutes.md) in the states of ADR-0112 §1.
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

/** The server's muted domains of the viewer, in the order of the domains, which POST and DELETE change. */
function mutedDomains(initial: string[], next_cursor?: string) {
  let domains = initial;
  const page = () =>
    HttpResponse.json({
      domain_mutes: domains.map((domain): Schemas["DomainMute"] => ({ domain, created_at: "2026-10-21T09:30:00Z" })),
      ...(next_cursor === undefined ? {} : { next_cursor }),
    } satisfies Schemas["DomainMutePage"]);
  return [
    http.get(`${apiBase}/domain_mutes`, ({ request }) => withViewer(request, page)),
    http.post(`${apiBase}/domain_mutes`, async ({ request }) => {
      const { domain } = (await request.clone().json()) as Schemas["MuteDomainRequest"];
      return withViewer(request, () => {
        domains = [...new Set([...domains, domain.toLowerCase()])].toSorted();
        return HttpResponse.json({ domain: domain.toLowerCase(), created_at: "2026-10-29T09:30:00Z" });
      });
    }),
    http.delete(`${apiBase}/domain_mutes/:domain`, ({ request, params }) =>
      withViewer(request, () => {
        domains = domains.filter((domain) => domain !== params["domain"]);
        return new HttpResponse(null, { status: 204 });
      }),
    ),
  ];
}

const meta = {
  title: "Screens/Settings/DomainMutes",
  render: renderScreen,
  loaders: [screenLoader],
  parameters: {
    screen: {
      path: "/settings/domain-mutes",
      signedIn: true,
      handlers: mutedDomains(["ads.example", "xn--r8jz45g.xn--zckzah"]),
    },
  },
  play: settled,
} satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** Two muted domains, with their count beside the heading and the button that mutes another. */
export const Normal: Story = {
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByText("2 muted domains")).toBeInTheDocument();
  },
};

/**
 * The form that mutes a domain, open in place of the button with focus in its field. (The unit tests submit it: the
 * submit button leaves with the panel, and React Aria's announcement of its pending state would name a gone element.)
 */
export const Adding: Story = {
  play: async (context) => {
    const { canvas, userEvent } = context;
    await settled(context);
    await userEvent.click(canvas.getByRole("button", { name: "Mute a domain" }));
    const field = canvas.getByRole("textbox", { name: /Domain/ });
    await expect(field).toHaveFocus();
    await userEvent.type(field, "spam.example");
  },
};

/** More muted domains than the first page: the count is a lower bound. */
export const MorePages: Story = answering(...mutedDomains(["ads.example", "spam.example"], "next"));

/** No muted domain: the empty state leads to muting one. */
export const Empty: Story = {
  ...answering(...mutedDomains([])),
  play: async (context) => {
    await settled(context);
    await expect(context.canvas.getByRole("button", { name: "Mute a domain" })).toBeInTheDocument();
  },
};

/** The muted domains have not arrived yet. */
export const Loading: Story = loadingOn("get", "/domain_mutes");

/** The server failed to answer the muted domains. */
export const ServerError: Story = failingOn("get", "/domain_mutes");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** In the accented pseudo-locale, whose longer words show where the layout runs out of room. */
export const PseudoLocale: Story = inLocale("en-XA");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px. */
export const Narrow: Story = narrow;

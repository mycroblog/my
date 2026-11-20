// SPDX-License-Identifier: AGPL-3.0-only

// The webhooks (FR-722, ADR-0060), on the fake server (app.samples.tsx; staff-webhooks.ts) as the administrator (hana)
// reads it, whose permissions lack `manage_errors`, so the errors' events are not offered: the list (an endpoint that
// works, one failing now, one the server turned off after its deliveries kept failing, one an administrator turned
// off), none, loading, failing, and the address for a moderator, whose permissions do not open it. The form of a new
// one, for the administrator and for a community lead (`lead`: no `manage_domains`, so the servers' events are left
// out too). A webhook of each state with its deliveries, and one with what each delivery sent shown; the lead on a
// webhook that receives an event they may not handle, shown with no form, and what its delivery of that event sent not
// shown to them; loading, failing, and an address that names none. The views of a secret shown once (a new
// webhook's, a new secret) follow a press: the app opens them through the feature's specimen (`WebhookSecretSpecimen`).

import { WebhookSecretSpecimen } from "@my/app-features";
import { PreviewExpanded } from "@my/design-system/preview";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Administration/Webhooks",
  ...screenMeta,
  parameters: {
    ...screenMeta.parameters,
    ...screen({ at: "/admin/webhooks", viewer: "hana" }),
  },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the administrator reads it. */
const asAdmin = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** The screen at `at` as a community lead reads it: hana holding only the role `lead`. */
const asLead = (path: string) =>
  screen({
    at: path,
    viewer: "hana",
    server: { staffServer: { roles: { viewer: ["lead"] } } },
  });

/** Every webhook, newest first: its host, what it is for, its events and where it stands. */
export const Webhooks: Story = { parameters: asAdmin("/admin/webhooks") };
/** No webhook yet: the sentence, and Set up a webhook. */
export const WebhooksNone: Story = {
  parameters: asAdmin("/admin/webhooks", {
    server: { staffServer: { webhooks: { none: true } } },
  }),
};
export const WebhooksLoading: Story = {
  parameters: asAdmin("/admin/webhooks", { state: "loading" }),
};
export const WebhooksError: Story = {
  parameters: asAdmin("/admin/webhooks", { state: "error" }),
};
/** The webhooks' address, for a moderator, whose permissions do not open it. */
export const WebhooksNotOpen: Story = {
  parameters: screen({ at: "/admin/webhooks", viewer: "marco" }),
};

/** A new webhook for the administrator: every event but the errors', which their roles leave out. */
export const NewWebhook: Story = { parameters: asAdmin("/admin/webhooks/new") };
/** A new webhook for the lead: the servers' and the errors' events left out, and named. */
export const NewWebhookForALead: Story = {
  parameters: asLead("/admin/webhooks/new"),
};

/** A webhook that works: On, Send a test, its deliveries (waiting, failing, delivered, a test). */
export const Webhook: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:alerts}"),
};
/** The same webhook with what each delivery sent shown under it, laid out as JSON, and the button that copies it. */
export const WebhookDeliveryBodies: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:alerts}"),
  render: (args, context) => <PreviewExpanded>{screenMeta.render(args, context)}</PreviewExpanded>,
};
/** The same webhook with Show more pressed once under its deliveries, two a page: all four, and Show more gone. */
export const WebhookMore: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:alerts}", { server: { staffPage: 2 }, more: true }),
};
/** A webhook whose deliveries are failing now, with how many and since when, and an address too long for a phone. */
export const WebhookFailing: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:chat}"),
};
/** A webhook the server turned off after its deliveries kept failing: Turn on, no test. */
export const WebhookTurnedOffByTheServer: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:archive}"),
};
/** A webhook an administrator turned off, with no delivery in the last 14 days. */
export const WebhookOff: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:old}"),
};
/**
 * The lead on a webhook that receives a server's moderation, whose permission they lack: what it is and receives, and
 * why they cannot change it, with no form; its test, its state, its secret and Delete are still theirs; what its
 * delivery of a server's moderation sent is not shown to them, and the row says why.
 */
export const WebhookForALead: Story = {
  parameters: asLead("/admin/webhooks/{server:webhook:chat}"),
};
export const WebhookLoading: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:alerts}", {
    state: "loading",
  }),
};
export const WebhookError: Story = {
  parameters: asAdmin("/admin/webhooks/{server:webhook:alerts}", {
    state: "error",
  }),
};
/** An address that names no webhook. */
export const WebhookMissing: Story = {
  parameters: asAdmin("/admin/webhooks/none"),
};

/** A secret of the server's form and length, so that its box shows how it wraps. */
const shownSecret = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw3b7Qf9eYkVk=";

/**
 * The whole app at `fake`'s address, its webhook screens opened with `shownSecret` as if the server had just given it:
 * a state only a press reaches, which the feature's specimen opens.
 */
const withSecret = (fake: ReturnType<typeof asAdmin>): Story => ({
  parameters: fake,
  render: (args, context) => (
    <WebhookSecretSpecimen value={shownSecret}>{screenMeta.render(args, context)}</WebhookSecretSpecimen>
  ),
});

/** A new webhook just set up: what happened and its secret, shown once, on one surface, and Done. */
export const SecretOfANewWebhook: Story = withSecret(asAdmin("/admin/webhooks/new"));
/** A webhook's page once a new secret is made: the secret at its top, shown once, over the rest of the page. */
export const NewSecretShownOnce: Story = withSecret(asAdmin("/admin/webhooks/{server:webhook:alerts}"));

// SPDX-License-Identifier: AGPL-3.0-only

// The webhooks (FR-722, ADR-0060), on the fake server (app.samples.tsx; staff-webhooks.ts) as the administrator (hana)
// reads it, whose permissions lack `manage_errors`, so the errors' events are not offered: the list (an endpoint that
// works, one failing now, one the server turned off after its deliveries kept failing, one an administrator turned
// off), none, loading, failing, and the address for a moderator, whose permissions do not open it. The form of a new
// one, for the administrator and for a community lead (`lead`: no `manage_domains`, so the servers' events are left
// out too). A webhook of each state with its deliveries; the lead on a webhook that receives an event they may not
// change, named and kept; loading, failing, and an address that names none. The views of a secret shown once (a new
// webhook's, a new secret) follow a press, so they are the feature's screens alone, opened in that state on a fake server of their own.

import { AppDataProvider, createAppData } from "@my/app-data";
import { NewWebhookScreen, WebhookScreen } from "@my/app-features";
import { createFakeApi, fakeOrigin } from "@my/e2e/fake";
import { createClient } from "@mycroblog/mykit";
import { FrameMain } from "@my/design-system";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
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
/** The lead on a webhook that receives an account's event, which needs a permission the lead holds, and the rest. */
export const WebhookForALead: Story = {
  parameters: asLead("/admin/webhooks/{server:webhook:archive}"),
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

/**
 * The feature's screen alone, on a fake server of its own as the administrator reads it, opened in a state only a
 * press reaches: a new webhook just set up, or a webhook's new secret just made, each secret shown once.
 */
function Specimen({ shows }: { shows: "created" | "rotated" }) {
  const [{ data, id, secret }] = useState(() => {
    const fake = createFakeApi({ viewer: "hana" });
    const data = createAppData(
      createClient({
        server: fakeOrigin,
        retry: { disabled: true },
        fetch: fake.serve,
      }),
    );
    // A secret of the server's form and length, so that its box shows how it wraps.
    return {
      data,
      id: fake.staffServer.id("webhook", "alerts"),
      secret: "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw3b7Qf9eYkVk=",
    };
  });
  const go = () => undefined;
  return (
    <AppDataProvider data={data}>
      <FrameMain>
        {shows === "created" ? (
          <NewWebhookScreen go={go} defaultMade={{ id, secret }} />
        ) : (
          <WebhookScreen id={id} go={go} defaultSecret={secret} />
        )}
      </FrameMain>
    </AppDataProvider>
  );
}

/** The view that takes the form's place once a new webhook is set up: what happened, then its secret, shown once. */
export const SecretOfANewWebhook: Story = {
  render: () => <Specimen shows="created" />,
};
/** A webhook's page once a new secret is made: the secret in the section's place, with the 24 hours it overlaps. */
export const NewSecretShownOnce: Story = {
  render: () => <Specimen shows="rotated" />,
};

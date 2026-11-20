// SPDX-License-Identifier: AGPL-3.0-only

// The facts of a record and the figures of a server's state (#3258): an account's registration, its email address
// and the IP address it registered from kept until shown, one being asked for, both shown, and Show answered by the
// server a moment later; a webhook's endpoint, its values in the code's face; the facts inside a raised section; and
// the figures of the dashboard, each leading to where it is dealt with, and those of a reader who may deal with none
// of them. The words are samples in English, as the moderation and administration screens would write them.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState, type ReactNode } from "react";
import { TextButton } from "./button";
import { CodeText } from "./code-text";
import { FactList, FigureTiles, type Fact, type Figure } from "./facts";
import { Stack } from "./layout";
import { GroupHeading } from "./list";
import { SettingsSection } from "./settings";

const rem = (px: number) => `${px / rootPx}rem`;
const none = () => undefined;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

const email = "river.tanaka@mail.example";
const ip = "203.0.113.42";

/** An account's registration; its email address and IP address as `kept` says. */
const registration = (kept: { email?: "kept" | "asked" | "shown"; ip?: "kept" | "asked" | "shown" }): Fact[] => [
  { term: "Account", value: "@river_tanaka" },
  { term: "Registered", value: "Nov 3, 2026, 9:41 PM" },
  {
    term: "Email",
    ...(kept.email === "shown" ? { value: email } : {}),
    reveal: { onReveal: none, busy: kept.email === "asked" },
  },
  {
    term: "Registered from",
    code: true,
    ...(kept.ip === "shown" ? { value: ip } : {}),
    reveal: { onReveal: none, busy: kept.ip === "asked" },
  },
  { term: "Invited by", value: "@mika" },
  { term: "Reason given", value: "I run a small bakery in Kyoto and would like to post about bread and the seasons." },
];

const meta = { title: "Facts", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** An account's registration: its email address and IP address kept, Show in their place. */
export const Registration: Story = {
  render: () => (
    <Column>
      <FactList label="Registration" items={registration({ email: "kept", ip: "kept" })} />
    </Column>
  ),
};

/** The email address being asked for, Show spinning; the IP address shown, in the code's face. */
export const RevealAsked: Story = {
  render: () => (
    <Column>
      <FactList label="Registration" items={registration({ email: "asked", ip: "shown" })} />
    </Column>
  ),
};

/** Both shown. */
export const Revealed: Story = {
  render: () => (
    <Column>
      <FactList label="Registration" items={registration({ email: "shown", ip: "shown" })} />
    </Column>
  ),
};

/** Show answered by the server a moment later, as the screen would hold it: the value then takes the focus. */
export const RevealOnPress: Story = {
  render: () => {
    function Facts() {
      const [state, setState] = useState<"kept" | "asked" | "shown">("kept");
      const items = registration({ email: state, ip: "kept" }).map((fact) =>
        fact.term === "Email"
          ? {
              ...fact,
              reveal: {
                busy: state === "asked",
                onReveal: () => {
                  setState("asked");
                  setTimeout(() => {
                    setState("shown");
                  }, 800);
                },
              },
            }
          : fact,
      );
      return <FactList label="Registration" items={items} />;
    }
    return (
      <Column>
        <Facts />
      </Column>
    );
  },
};

/** A webhook's endpoint: the URL and the secret's fingerprint in the code's face, breaking anywhere. */
export const Endpoint: Story = {
  render: () => (
    <Column>
      <FactList
        label="Endpoint"
        items={[
          { term: "URL", code: true, value: "https://hooks.example.com/mycroblog/moderation/7f3a9c2e41d8b6f0" },
          { term: "Events", value: "report.created, account.suspended, account.approved" },
          { term: "Signing secret", code: true, value: "sha256:9f86d081…b0f00a08" },
          { term: "Created", value: "Oct 12, 2026 by @admin_sato" },
          { term: "Last delivery", value: "Failed 12 minutes ago (HTTP 500)" },
        ]}
      />
    </Column>
  ),
};

/** A job's keys: each in the code's face, left to right in any page, with the button that copies it after it. */
export const WithActions: Story = {
  render: () => (
    <Column>
      <FactList
        label="The job"
        items={[
          {
            term: "ID",
            code: true,
            value: "019a6d2e-4b1c-7f3a-8c2e-41d8b6f07a3c",
            action: <TextButton onPress={none}>Copy the ID</TextButton>,
          },
          {
            term: "Idempotency key",
            code: true,
            value: "notification.email:01J9YZ8Q1A2B3C4D5E6F7G8H9J",
            action: <TextButton onPress={none}>Copy the key</TextButton>,
          },
          { term: "Worker", code: true, value: "worker-7f3a" },
          { term: "Type", value: <CodeText>notification.email</CodeText> },
        ]}
      />
    </Column>
  ),
};

/** The registration in a column narrower than a phone's (a detail inside a log's entry): each term over its value. */
export const NarrowColumn: Story = {
  render: () => (
    <div style={{ maxWidth: rem(208), padding: rem(16) }}>
      <FactList label="Registration" items={registration({ email: "shown", ip: "shown" })} />
    </div>
  ),
};

/** The facts inside a raised section, under its heading, Show on the page's surface. */
export const InASection: Story = {
  render: () => (
    <Column>
      <SettingsSection heading="Registration">
        <FactList label="Registration" items={registration({ email: "kept", ip: "shown" })} />
      </SettingsSection>
    </Column>
  ),
};

const figures: readonly Figure[] = [
  { figure: "12", label: "Open reports", href: "/moderation/reports" },
  { figure: "3", label: "Pending registrations", href: "/admin/registrations" },
  { figure: "1,204", label: "Jobs in the queue", href: "/admin/jobs" },
  { figure: "2", label: "Error groups this week", href: "/admin/errors" },
  { figure: "17", label: "Servers failing deliveries for over a day", href: "/admin/instances" },
];

/** The dashboard's figures, each leading to where it is dealt with. */
export const Figures: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-line">
        <GroupHeading>This server</GroupHeading>
        <FigureTiles label="This server" items={figures} />
      </Stack>
    </Column>
  ),
};

/** The figures of a reader who may deal with none of them: each on the page, in the same room, leading nowhere. */
export const FiguresUnlinked: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-line">
        <GroupHeading>This server</GroupHeading>
        <FigureTiles label="This server" items={figures.map(({ figure, label }) => ({ figure, label }))} />
      </Stack>
    </Column>
  ),
};

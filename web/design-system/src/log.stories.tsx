// SPDX-License-Identifier: AGPL-3.0-only

// The logs of a record (#3258), the newest first: the audit log, its entries folded and one opened; a webhook's
// deliveries, a failed one opened on its request's payload and its response; a job's attempts; and a block of code to
// copy, alone: a payload long enough to scroll in place, a signing secret shown once, and the errors of an import. The
// words are samples in English, as the moderation and administration screens would write them.

import { rootPx } from "@my/design-tokens";
import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { Button } from "./button";
import { FactList } from "./facts";
import { Stack } from "./layout";
import { GroupHeading, SentenceName } from "./list";
import { LogEntry, LogList } from "./log";
import { PreviewExpanded } from "./preview";
import { ago } from "./samples";
import { CopyBlock } from "./settings";

const rem = (px: number) => `${px / rootPx}rem`;
const minute = 60_000;
const hour = 60 * minute;

/** The column of a screen: as wide as the app's main column at most, the frame's edge around it. */
function Column({ children }: { children: ReactNode }) {
  return <div style={{ maxWidth: rem(600), padding: rem(16) }}>{children}</div>;
}

/** The audit log's latest entries; `open`, their details shown. */
function Audit() {
  return (
    <LogList label="Audit log">
      <LogEntry
        icon="moderation"
        date={ago(4 * minute)}
        about="Reason: Spam. Resolves 41 reports."
        detail={
          <FactList
            label="Suspension"
            items={[
              { term: "Action", value: "Suspend" },
              { term: "Account", value: "@spam_bot_7731@social.example" },
              { term: "Reports", value: "2291, 2290 and 39 more" },
              { term: "Notified", value: "No: the account's server is suspended" },
            ]}
          />
        }
      >
        <SentenceName>mika</SentenceName> suspended <SentenceName>@spam_bot_7731@social.example</SentenceName>
      </LogEntry>
      <LogEntry icon="hidden" date={ago(38 * minute)} about="Email address and IP address">
        <SentenceName>admin_sato</SentenceName> showed the private data of <SentenceName>@river_tanaka</SentenceName>
      </LogEntry>
      <LogEntry
        icon="settings"
        date={ago(3 * hour)}
        detail={
          <FactList
            label="Change"
            items={[
              { term: "Before", value: "Open" },
              { term: "After", value: "Approval required" },
            ]}
          />
        }
      >
        <SentenceName>admin_sato</SentenceName> changed <SentenceName>Registrations</SentenceName> to Approval required
      </LogEntry>
      <LogEntry icon="accepted" date={ago(26 * hour)} about="Appeal 318 accepted">
        <SentenceName>mika</SentenceName> lifted the limit on <SentenceName>@kenji@photos.example</SentenceName>
      </LogEntry>
      <LogEntry icon="block" date={ago(3 * 24 * hour)} about="Reason: Harassment, coordinated from this server">
        <SentenceName>yuki</SentenceName> suspended the domain <SentenceName>bad-actors.example</SentenceName>
      </LogEntry>
    </LogList>
  );
}

/** A request's payload for an event about a report or an account. */
const payloadOf = (event: string, object: Record<string, unknown>) =>
  JSON.stringify({ event, created_at: "2026-11-20T01:02:44Z", object }, null, 2);

const failed = `HTTP/1.1 500 Internal Server Error
content-type: text/plain

upstream timed out while reading response header from https://hooks.example.com/mycroblog/moderation/7f3a9c2e41d8b6f0`;

const delivered = "HTTP/1.1 204 No Content";

/** A delivery's request and its response. */
function Delivery({ payload, response }: { payload: string; response: string }) {
  return (
    <Stack gap="screen-row-gap">
      <CopyBlock label="Payload" code={payload} action={<Button size="timeline">Copy payload</Button>} />
      <CopyBlock label="Response" code={response} action={<Button size="timeline">Copy response</Button>} />
    </Stack>
  );
}

const report = payloadOf("report.created", {
  id: "2291",
  category: "spam",
  comment: "Posts the same link to a crypto giveaway in every reply.",
  account: { id: "1092", acct: "reporter_aoi" },
  target_account: { id: "88213", acct: "spam_bot_7731@social.example" },
  status_ids: ["113402998127734913", "113402998127734920"],
});

/** A webhook's deliveries. */
function Deliveries() {
  return (
    <LogList label="Deliveries">
      <LogEntry
        icon="error"
        date={ago(12 * minute)}
        about="Failed: HTTP 500 after 10.0 s. Attempt 3 of 5, next in 8 min."
        detail={<Delivery payload={report} response={failed} />}
      >
        <SentenceName>report.created</SentenceName> to hooks.example.com
      </LogEntry>
      <LogEntry
        icon="accepted"
        date={ago(2 * hour)}
        about="Delivered: HTTP 204 in 182 ms"
        detail={
          <Delivery
            payload={payloadOf("account.suspended", { id: "88213", acct: "spam_bot_7731@social.example" })}
            response={delivered}
          />
        }
      >
        <SentenceName>account.suspended</SentenceName> to hooks.example.com
      </LogEntry>
      <LogEntry
        icon="accepted"
        date={ago(5 * hour)}
        about="Delivered: HTTP 204 in 240 ms"
        detail={
          <Delivery
            payload={payloadOf("account.approved", { id: "90412", acct: "river_tanaka" })}
            response={delivered}
          />
        }
      >
        <SentenceName>account.approved</SentenceName> to hooks.example.com
      </LogEntry>
    </LogList>
  );
}

const trace = `delivery: POST https://social.example/inbox: dial tcp 198.51.100.7:443: connect: connection refused
  at internal/federation/deliver.go:214 (deliverOne)
  at internal/jobs/worker.go:88 (run)`;

const meta = { title: "Logs", tags: ["themes"] } satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The audit log, the newest first: who did what to what, the time at the end, Details where there is more. */
export const AuditLog: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-line">
        <GroupHeading>Audit log</GroupHeading>
        <Audit />
      </Stack>
    </Column>
  ),
};

/** The audit log with every entry's details shown. */
export const AuditLogOpen: Story = {
  render: () => (
    <Column>
      <PreviewExpanded>
        <Audit />
      </PreviewExpanded>
    </Column>
  ),
};

/** A webhook's deliveries, folded: the failed one first, its status and its next attempt under it. */
export const WebhookDeliveries: Story = {
  render: () => (
    <Column>
      <Deliveries />
    </Column>
  ),
};

/** The deliveries with their details shown: each request's payload and its response, with their copy actions. */
export const WebhookDeliveriesOpen: Story = {
  render: () => (
    <Column>
      <PreviewExpanded>
        <Deliveries />
      </PreviewExpanded>
    </Column>
  ),
};

/** A job's attempts: two that failed, then the one that succeeded. */
export const JobAttempts: Story = {
  render: () => (
    <Column>
      <LogList label="Attempts">
        <LogEntry icon="accepted" date={ago(2 * minute)} about="Took 1.4 s">
          Attempt 3 delivered <SentenceName>Create</SentenceName> to social.example
        </LogEntry>
        <LogEntry
          icon="error"
          date={ago(9 * minute)}
          about="dial tcp 198.51.100.7:443: connect: connection refused"
          detail={<CopyBlock label="Error" code={trace} action={<Button size="timeline">Copy error</Button>} />}
        >
          Attempt 2 failed
        </LogEntry>
        <LogEntry icon="error" date={ago(10 * minute)} about="context deadline exceeded after 30 s">
          Attempt 1 failed
        </LogEntry>
      </LogList>
    </Column>
  ),
};

const longPayload = JSON.stringify(
  {
    event: "account.suspended",
    created_at: "2026-11-20T00:41:09Z",
    object: {
      id: "88213",
      username: "spam_bot_7731",
      domain: "social.example",
      note: "Win 10 ETH today! Follow and reply to enter. https://giveaway.example/claim?ref=spam_bot_7731&utm_source=fediverse&utm_medium=reply",
      suspended_at: "2026-11-20T00:41:09Z",
      reports: Array.from({ length: 12 }, (_, i) => ({ id: String(2291 - i), category: "spam" })),
    },
  },
  null,
  2,
);

/**
 * Blocks of code to copy, alone: a payload longer than its room, which scrolls in place, a line of it wrapped; a
 * signing secret shown once, with what came of copying; and the errors of an import.
 */
export const CodeBlocks: Story = {
  render: () => (
    <Column>
      <Stack gap="entry-room">
        <CopyBlock label="Payload" code={longPayload} action={<Button size="timeline">Copy payload</Button>} />
        <CopyBlock
          label="Signing secret"
          values={["whsec_4f9b2c7e1a8d6305b9e2f71c4a0d8e36"]}
          action={<Button size="timeline">Copy secret</Button>}
          result="Copied"
        />
        <CopyBlock
          label="Errors"
          code={[
            "line 14: @kenji@photos.example: no such account",
            "line 15: @former_member: the account was deleted",
            "line 31: bad-actors.example is already suspended",
          ].join("\n")}
          action={<Button size="timeline">Copy errors</Button>}
        />
      </Stack>
    </Column>
  ),
};

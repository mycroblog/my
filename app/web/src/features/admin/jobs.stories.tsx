// SPDX-License-Identifier: AGPL-3.0-only
// The queue dashboard (/admin/jobs; web-client/screens/admin-jobs.md) in the states of ADR-0112 §1.
import type { Meta, StoryObj } from "@storybook/react-vite";
import { http, HttpResponse } from "msw";
import { expect } from "storybook/test";
import type { Schemas } from "../../test/fixtures";
import { apiBase, withViewer } from "../../test/msw/http";
import { inLocale, narrow, settled } from "../../test/stories";
import { adminAnswering, adminFailingOn, adminLoadingOn, adminScreen, withoutPermission } from "./test/stories";

const hour = 60 * 60 * 1000;

/** An instant `hours` before now, so the waits and ages read the same whenever the story runs. */
function ago(hours: number): string {
  return new Date(Date.now() - hours * hour).toISOString();
}

/** The jobs of a busy server: deliveries waiting and retrying, media in progress, and failures of several kinds. */
function jobsFixture(): Record<"pending" | "dead", Schemas["AdminJob"][]> {
  const pending: Schemas["AdminJob"][] = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000e2",
      type: "media.process",
      state: "running",
      payload: '{"media_id":"0192f0c4-7a8e-7000-8000-00000000m001"}',
      attempts: 1,
      run_at: ago(0.1),
      worker: "worker-7f3a",
      locked_until: new Date(Date.now() + 0.05 * hour).toISOString(),
      created_at: ago(0.1),
    },
    ...Array.from({ length: 6 }, (_, index): Schemas["AdminJob"] => ({
      id: `0192f0c4-7a8e-7000-8000-0000000000f${String(index)}`,
      type: "federation.deliver",
      state: index % 2 === 0 ? "retrying" : "queued",
      payload: `{"delivery_id":"0192f0c4-7a8e-7000-8000-00000000d00${String(index)}","inbox":"https://very-long-instance-name-${String(index)}.social.example/inbox"}`,
      ordering_key: `inbox:very-long-instance-name-${String(index)}.social.example`,
      attempts: index % 2 === 0 ? index + 2 : 0,
      ...(index % 2 === 0 && { last_error: "503 Service Unavailable" }),
      run_at: new Date(Date.now() + (index + 1) * 0.1 * hour).toISOString(),
      created_at: ago(index + 1),
    })),
  ];
  const dead: Schemas["AdminJob"][] = [
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000d1",
      type: "federation.deliver",
      state: "dead",
      payload: '{"delivery_id":"x"}',
      idempotency_key: "deliver:x",
      attempts: 9,
      last_error: "502 Bad Gateway: the upstream server answered with an HTML page instead of an ActivityPub document",
      created_at: ago(96),
      failed_at: ago(2),
    },
    {
      id: "0192f0c4-7a8e-7000-8000-0000000000d0",
      type: "kv.cleanup",
      state: "dead",
      payload: "null",
      attempts: 3,
      last_error: "db down",
      created_at: ago(30),
      failed_at: ago(29),
    },
  ];
  return { pending, dead };
}

/** The queue of a database adapter whose oldest due job has waited for 40 minutes. */
const queue: Schemas["AdminQueue"] = {
  adapter: "db",
  pending_listing: true,
  counts: [
    { state: "queued", type: "federation.deliver", count: 3 },
    { state: "running", type: "media.process", count: 1 },
    { state: "retrying", type: "federation.deliver", count: 3 },
    { state: "dead", type: "federation.deliver", count: 1 },
    { state: "dead", type: "kv.cleanup", count: 1 },
  ],
};

const busy = [
  http.get(`${apiBase}/admin/queue`, ({ request }) =>
    withViewer(request, () => HttpResponse.json({ ...queue, oldest_ready_at: ago(40 / 60) })),
  ),
  http.get(`${apiBase}/admin/jobs`, ({ request }) => {
    const jobs = jobsFixture();
    const items = new URL(request.url).searchParams.get("state") === "dead" ? jobs.dead : jobs.pending;
    return withViewer(request, () => HttpResponse.json({ items }));
  }),
  http.get(`${apiBase}/admin/jobs/:id/attempts`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({
        items: [
          {
            attempt: 2,
            type: "federation.deliver",
            outcome: "retry",
            error: "503 Service Unavailable",
            worker: "worker-1",
            started_at: ago(1),
            finished_at: ago(0.99),
          },
          {
            attempt: 1,
            type: "federation.deliver",
            outcome: "retry",
            error: "connection refused",
            worker: "worker-2",
            started_at: ago(2),
            finished_at: ago(1.99),
          },
        ],
      } satisfies Schemas["AdminJobAttemptPage"]),
    ),
  ),
];

const meta = { title: "Screens/Admin/Jobs", ...adminScreen("/admin/jobs", busy) } satisfies Meta;

export default meta;
type Story = StoryObj<typeof meta>;

/** A busy queue: deliveries retrying, a job running, and failures waiting to run again. */
export const Normal: Story = {};

/** The failed jobs, each with its last error. */
export const Failed: Story = {
  parameters: { screen: { path: "/admin/jobs?state=dead" } },
  play: async (context) => {
    await settled(context);
    await expect(await context.canvas.findByText(/502 Bad Gateway/u)).toBeInTheDocument();
  },
};

/** A queue without jobs (the default handlers). */
export const Empty: Story = adminAnswering();

/** A queue adapter that cannot list its waiting jobs. */
export const WithoutListing: Story = adminAnswering(
  http.get(`${apiBase}/admin/queue`, ({ request }) =>
    withViewer(request, () =>
      HttpResponse.json({ adapter: "sqs", pending_listing: false, counts: [] } satisfies Schemas["AdminQueue"]),
    ),
  ),
);

/** A moderator without manage_jobs. */
export const MissingPermission: Story = withoutPermission;

/** The jobs have not arrived yet. */
export const Loading: Story = adminLoadingOn("get", "/admin/jobs");

/** The server failed to answer the queue. */
export const ServerError: Story = adminFailingOn("get", "/admin/queue");

/** In Japanese. */
export const Japanese: Story = inLocale("ja");

/** Right to left, in the pseudo-locale ar-XB. */
export const RightToLeft: Story = inLocale("ar-XB");

/** At 320 CSS px, where the long inbox addresses wrap. */
export const Narrow: Story = narrow;

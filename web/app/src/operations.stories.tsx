// SPDX-License-Identifier: AGPL-3.0-only

// The server's operations (ADR-0004, ADR-0062, ADR-0103 §8, ADR-0167 §3, ADR-0058 §1), on the fake server
// (app.samples.tsx) as its administrator (hana) reads them. Jobs: the queue's state, by state and by type; the list of
// the jobs that are not finished, of the failed ones and of one type, none, loading and failing; a failed job with its
// attempts, one waiting to retry, one running, and an address that names none. Errors: the open groups, every group,
// none, loading and failing; a panic of the server with its latest occurrence, an error of the web client, a group
// resolved after it happened again, a muted one, and an address that names none. UX measurements: the last 24 hours and
// the last 7 days, every session measured, measuring off, loading. Link card domains: the rules, none, a new rule, a rule, and an address that
// names none. What follows a press (the question before Run again, resolved, a refused domain) is the route tests' and
// the E2E's.

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, screenMeta, type Fake } from "./app.samples";

const meta = {
  title: "Operations",
  ...screenMeta,
  parameters: { ...screenMeta.parameters, ...screen({ at: "/admin/jobs", viewer: "hana" }) },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

/** The screen at `at` as the scenario's administrator reads it. */
const at = (path: string, more: Omit<Fake, "at"> = {}) => screen({ at: path, viewer: "hana", ...more });

/** Jobs: how many are in each state, the oldest due job, and the jobs by type. */
export const Jobs: Story = { parameters: at("/admin/jobs") };
/** Jobs on a queue with none. */
export const JobsEmpty: Story = { parameters: at("/admin/jobs", { server: { operations: "none" } }) };
export const JobsLoading: Story = { parameters: at("/admin/jobs", { state: "loading" }) };
export const JobsError: Story = { parameters: at("/admin/jobs", { state: "error" }) };

/** The jobs that are not finished, the earliest due first. */
export const JobList: Story = { parameters: at("/admin/jobs/list") };
/** The jobs' second page, two a page. */
export const JobListSecondPage: Story = {
  parameters: at("/admin/jobs/list", { server: { staffPage: 2 }, next: true }),
};
/** The failed jobs, the most recently failed first. */
export const JobListFailed: Story = { parameters: at("/admin/jobs/list?state=dead") };
/** The jobs of one type. */
export const JobListType: Story = { parameters: at("/admin/jobs/list?type=federation.deliver") };
/** No job waits or runs. */
export const JobListEmpty: Story = { parameters: at("/admin/jobs/list", { server: { operations: "none" } }) };
export const JobListLoading: Story = { parameters: at("/admin/jobs/list", { state: "loading" }) };
export const JobListError: Story = { parameters: at("/admin/jobs/list", { state: "error" }) };

/** A failed job: Run again, its facts, its last error, its payload and its one attempt. */
export const JobFailed: Story = { parameters: at("/admin/jobs/{ops:job:dead-email}") };
/** A delivery that used up its eight attempts. */
export const JobFailedAttempts: Story = { parameters: at("/admin/jobs/{ops:job:dead-deliver}") };
/** A failed job's runs with Show more pressed once, two a page. */
export const JobAttemptsMore: Story = {
  parameters: at("/admin/jobs/{ops:job:dead-deliver}", { server: { staffPage: 2 }, more: true }),
};
/** A job waiting to retry after three failures. */
export const JobRetrying: Story = { parameters: at("/admin/jobs/{ops:job:retrying}") };
/** A job a worker runs. */
export const JobRunning: Story = { parameters: at("/admin/jobs/{ops:job:running}") };
/** An address that names no job in the queue. */
export const JobMissing: Story = { parameters: at("/admin/jobs/0190d1a2-0000-7000-8000-000000000000") };
export const JobLoading: Story = { parameters: at("/admin/jobs/{ops:job:dead-email}", { state: "loading" }) };

/** Errors: the open groups, the most recently seen first. */
export const Errors: Story = { parameters: at("/admin/errors") };
/** Every group, of every state. */
export const ErrorsAll: Story = { parameters: at("/admin/errors?state=any") };
/** The error groups' second page, one a page. */
export const ErrorsSecondPage: Story = {
  parameters: at("/admin/errors", { server: { staffPage: 1 }, next: true }),
};
/** No error is open. */
export const ErrorsEmpty: Story = { parameters: at("/admin/errors", { server: { operations: "none" } }) };
export const ErrorsLoading: Story = { parameters: at("/admin/errors", { state: "loading" }) };
export const ErrorsError: Story = { parameters: at("/admin/errors", { state: "error" }) };

/** A panic of the server: Resolve and Mute, its facts, its latest occurrence with its IDs and stack, its hours. */
export const ErrorGroup: Story = { parameters: at("/admin/errors/{ops:error:panic}") };
/** The same group with occurrences over 30 hours: the newest 12, and Show more. */
export const ErrorGroupManyHours: Story = {
  parameters: at("/admin/errors/{ops:error:panic}", { server: { operations: "crowded" } }),
};
/** An error of the web client, without a request ID. */
export const ErrorGroupWeb: Story = { parameters: at("/admin/errors/{ops:error:web}") };
/** A group resolved, regressed on another version, and resolved again: Mute, and its history. */
export const ErrorGroupResolved: Story = { parameters: at("/admin/errors/{ops:error:resolved}") };
/** A muted group: Unmute. */
export const ErrorGroupMuted: Story = { parameters: at("/admin/errors/{ops:error:muted}") };
/** An address that names no error group. */
export const ErrorGroupMissing: Story = { parameters: at("/admin/errors/0190d1a2-0000-7000-8000-000000000000") };

/** UX measurements of the last 24 hours, measuring on: a table of each metric. */
export const UXMeasurements: Story = { parameters: at("/admin/ux-measurements", { server: { uxMeasurement: true } }) };
/** The last 7 days. */
export const UXMeasurementsWeek: Story = {
  parameters: at("/admin/ux-measurements?hours=168", { server: { uxMeasurement: true } }),
};
/** Every browser session measured (`ux_measurement_share` 100): the share's field under the switch says 100 %. */
export const UXMeasurementsEverySession: Story = {
  parameters: at("/admin/ux-measurements", { server: { uxMeasurement: true, uxMeasurementShare: 100 } }),
};
/** Measuring off, the server's default: no samples, and the setting under them. */
export const UXMeasurementsOff: Story = { parameters: at("/admin/ux-measurements") };
export const UXMeasurementsLoading: Story = { parameters: at("/admin/ux-measurements", { state: "loading" }) };

/** Link card domains: the rules, a door each, and Add a rule. */
export const LinkCardDomains: Story = { parameters: at("/admin/link-card-domains") };
/** More rules than the screen shows at once: the first 40, and Show more. */
export const LinkCardDomainsMany: Story = {
  parameters: at("/admin/link-card-domains", { server: { operations: "crowded" } }),
};
/** No domain has a rule. */
export const LinkCardDomainsEmpty: Story = {
  parameters: at("/admin/link-card-domains", { server: { operations: "none" } }),
};
/** A new rule: its domain, its mode and a note. */
export const LinkCardDomainNew: Story = { parameters: at("/admin/link-card-domains/new") };
/** A rule: its mode and note as a form, and Remove last. */
export const LinkCardDomain: Story = { parameters: at("/admin/link-card-domains/tracker.example") };
/** An address that names a domain without a rule. */
export const LinkCardDomainMissing: Story = { parameters: at("/admin/link-card-domains/none.example") };

// SPDX-License-Identifier: AGPL-3.0-only

// The server's operations on the real server (#3265, ADR-0004, ADR-0103 §8): an administrator finds the failed jobs
// under Jobs, by the figure of the failed ones, opens a job that failed for good, reads why it failed and its attempt,
// and runs it again, which asks first; and finds an error the web client reported among the open errors by its
// message, resolves it, and no longer finds it among them. The job is put in the queue with a payload its handler cannot
// read by the test build's `my dev dead-job`, which the server's own worker fails at once, and deleted at the end once
// it failed again; the error's message is its own, so that a repeat of this file on another worker finds its own. A
// resolved group stays on the server, which deletes none. What each screen shows in every state is the stories' and the
// route tests' on the fake (web/app, operations.stories.tsx).

import { Operations } from "./screens";
import { expect, test } from "./test";

test("an administrator finds a failed job under Jobs, reads why it failed, and runs it again", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  let key: string | undefined;
  try {
    key = fresh.failingJob();
    const job = await fresh.deadJob(admin, key);
    await signIn(admin);
    const ops = new Operations(page);
    await ops.openEntry("Jobs");
    await ops.openState("Failed");
    await expect(ops.jobsOf(job.type)).not.toHaveCount(0);

    await ops.openJob(job.type, job.id);
    await expect(ops.jobFacts).toContainText("Failed");
    await expect(ops.attempts).toHaveCount(1);

    await ops.runAgain();
    await expect(ops.said("The job is queued to run again.")).toBeVisible();
  } finally {
    if (key !== undefined) await fresh.forgetJob(admin, key);
  }
});

test("an administrator resolves an error the web client reported, and it leaves the open errors", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  const error = await fresh.clientError(admin);
  await signIn(admin);
  const ops = new Operations(page);
  await ops.openEntry("Errors");
  await expect(ops.group(error.message)).toBeVisible();

  await ops.openGroup(error.message, error.type);
  await expect(ops.groupFacts).toContainText("Open");
  await ops.resolve();
  await expect(ops.said("The error is resolved.")).toBeVisible();
  await expect(ops.groupFacts).toContainText("Resolved");

  await ops.backToGroups();
  await expect(ops.groups).toBeVisible();
  await expect(ops.group(error.message)).toHaveCount(0);
});

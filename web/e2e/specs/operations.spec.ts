// SPDX-License-Identifier: AGPL-3.0-only

// The server's operations on the real server (#3265, ADR-0004, ADR-0103 §8): an administrator finds a job that failed
// for good under Jobs, by the figure of the failed ones, reads why it failed and its attempt, and runs it again, which
// asks first; and finds an error the web client reported among the open errors, resolves it, and no longer finds it
// among them. The job is put in the queue with a payload its handler cannot read, which the server's own worker fails
// at once (fresh.ts, `deadJob`), and deleted at the end once it failed again; the error has a name of its own, so that
// a repeat of this file on another worker finds its own. A resolved group stays on the server, which deletes none.
// What each screen shows in every state is the stories' and the route tests' on the fake (web/app,
// operations.stories.tsx).

import { Operations } from "./screens";
import { expect, test } from "./test";

test("an administrator finds a failed job under Jobs, reads why it failed, and runs it again", async ({
  page,
  signIn,
  fresh,
}) => {
  const admin = await fresh.person({ admin: true });
  const job = await fresh.deadJob(admin);
  try {
    await signIn(admin);
    const ops = new Operations(page);
    await ops.openEntry("Jobs");
    await ops.openState("Failed");
    await expect(ops.job(job.id)).toBeVisible();

    await ops.openJob(job.type, job.id);
    await expect(ops.jobFacts).toContainText("Failed");
    await expect(ops.attempts).toHaveCount(1);

    await ops.runAgain();
    await expect(ops.said("The job is queued to run again.")).toBeVisible();
  } finally {
    await fresh.forgetJob(admin, job.id);
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
  await expect(ops.group(error.type)).toBeVisible();

  await ops.openGroup(error.type);
  await expect(ops.groupFacts).toContainText("Open");
  await ops.resolve();
  await expect(ops.said("The error is resolved.")).toBeVisible();
  await expect(ops.groupFacts).toContainText("Resolved");

  await ops.backToGroups();
  await expect(ops.groups).toBeVisible();
  await expect(ops.group(error.type)).toHaveCount(0);
});

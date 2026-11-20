// SPDX-License-Identifier: AGPL-3.0-only

// The server's operations in the administration: the job queue and its failed jobs, a job and Run again; the error
// groups and a group, resolved. Each write returns once the server has answered it.

import { expect, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class Operations {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  private button(name: string): Locator {
    return this.main.getByRole("button", { name, exact: true });
  }

  /** Opens the screen of the entry `name` (Jobs, Errors) from the root of the administration, /admin. */
  async openEntry(name: "Jobs" | "Errors"): Promise<void> {
    await this.page.goto("/admin");
    await this.main
      .getByRole("navigation", { name: "Administration screens", exact: true })
      .getByRole("link", { name: new RegExp(`^${escape(name)} .+$`) })
      .click();
    await expect(this.main.getByRole("heading", { level: 1, name, exact: true })).toBeVisible();
  }

  /** Opens the jobs of the state `state` from the figure that counts them on the queue's screen. */
  async openState(state: "Queued" | "Running" | "Waiting to retry" | "Failed"): Promise<void> {
    await this.main.getByRole("link", { name: new RegExp(`^[\\d,]+ ${escape(state)}$`) }).click();
    await expect(this.jobs).toBeVisible();
  }

  /** The table of jobs of the list. */
  get jobs(): Locator {
    return this.main.getByRole("table", { name: "Jobs", exact: true });
  }

  /** The links of the list to the jobs of the type `type`. */
  jobsOf(type: string): Locator {
    return this.jobs.getByRole("link", { name: type, exact: true });
  }

  /** Opens the job of `id`, of the type `type`, at its address. */
  async openJob(type: string, id: string): Promise<void> {
    await this.page.goto(`/admin/jobs/${id}`);
    await expect(this.main.getByRole("heading", { level: 1, name: type, exact: true })).toBeVisible();
  }

  /** The facts of the job on its screen. */
  get jobFacts(): Locator {
    return this.main.getByLabel("The job", { exact: true });
  }

  /** The job's runs that did not succeed, a row each. */
  get attempts(): Locator {
    return this.main.getByRole("table", { name: "Failed attempts", exact: true }).getByRole("rowheader");
  }

  /** Runs the failed job of the screen again, as its question asks, and returns once the server answered. */
  async runAgain(): Promise<void> {
    await this.button("Run again").click();
    const dialog = this.page.getByRole("alertdialog", { name: "Run this job again?", exact: true });
    await expect(dialog).toBeVisible();
    await answered(this.page, { methods: ["POST"], path: /\/admin\/jobs\/[^/]+\/retry$/ }, async () => {
      await dialog.getByRole("button", { name: "Run again", exact: true }).click();
    });
  }

  /** The table of error groups. */
  get groups(): Locator {
    return this.main.getByRole("table", { name: "Error groups", exact: true });
  }

  /** The link of the list to the group whose message is `message`. */
  group(message: string): Locator {
    return this.groups.getByRole("link", { name: message, exact: true });
  }

  /** Opens the group whose message is `message`, of the type `type`, from the list. */
  async openGroup(message: string, type: string): Promise<void> {
    await this.group(message).click();
    await expect(this.main.getByRole("heading", { level: 1, name: type, exact: true })).toBeVisible();
  }

  /** The facts of the error group on its screen. */
  get groupFacts(): Locator {
    return this.main.getByLabel("The error", { exact: true });
  }

  /** Resolves the group of the screen, and returns once the server answered. */
  async resolve(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/admin\/error_groups\/[^/]+\/resolve$/ }, async () => {
      await this.button("Resolve").click();
    });
  }

  /** The way back from a group to the list of groups. */
  async backToGroups(): Promise<void> {
    await this.button("Errors").click();
    await expect(this.main.getByRole("heading", { level: 1, name: "Errors", exact: true })).toBeVisible();
  }

  /** What the screen says a press did, in its words. */
  said(words: string): Locator {
    return this.main.getByRole("status").getByText(words, { exact: true });
  }
}

// SPDX-License-Identifier: AGPL-3.0-only

// The moderators' screens of domains and the audit log: Domain actions in Moderation's list of screens,
// the form of a new domain action with the confirmation of a suspend (the follows it ends, the domain to type), the
// action's page, and the entries of the audit log. Each write returns once the server has answered it.

import type { Locator, Page } from "@playwright/test";
import { answered } from "./write";

export class StaffDomains {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  private get main(): Locator {
    return this.page.getByRole("main");
  }

  /** The list of the moderation screens the reader's permissions open, at /moderation. */
  private get screens(): Locator {
    return this.main.getByRole("navigation", { name: "Moderation screens", exact: true });
  }

  /** The root's link to Domain actions, with its hint. */
  get domainActionsLink(): Locator {
    return this.screens.getByRole("link", { name: /^Domain actions .+$/ });
  }

  /** Opens the form of a new domain action from the list. */
  async newAction(): Promise<void> {
    await this.main.getByRole("link", { name: "New action", exact: true }).click();
  }

  /** Fills the form of a new action: the domain and the public reason; a suspend until lifted is its start. */
  async fill(domain: string, reason: string): Promise<void> {
    await this.main.getByRole("textbox", { name: "Domain", exact: true }).fill(domain);
    await this.main.getByRole("textbox", { name: "Public reason", exact: true }).fill(reason);
  }

  /** Sends the form of a suspend, which asks first. */
  async suspend(): Promise<void> {
    await this.main.getByRole("button", { name: "Suspend…", exact: true }).click();
  }

  /** The confirmation of a suspend of `domain`. */
  confirmation(domain: string): Locator {
    return this.page.getByRole("alertdialog", { name: `Suspend ${domain}?`, exact: true });
  }

  /** Types the domain into the confirmation of its suspend and confirms, and returns once the server answered. */
  async confirmSuspend(domain: string): Promise<void> {
    const dialog = this.confirmation(domain);
    await dialog.getByRole("textbox").fill(domain);
    await answered(this.page, { methods: ["POST"], path: /\/admin\/domain_actions$/ }, async () => {
      await dialog.getByRole("button", { name: "Suspend", exact: true }).click();
    });
  }

  /** The heading of an action's page, its domain. */
  actionHeading(domain: string): Locator {
    return this.main.getByRole("heading", { level: 1, name: domain, exact: true });
  }

  /** The facts of an action on its page. */
  get actionFacts(): Locator {
    return this.main.getByLabel("About this action", { exact: true });
  }

  /** Opens the audit log of the operations on an action's domain, from the action's page. */
  async auditLogOfDomain(): Promise<void> {
    await this.main.getByRole("link", { name: "Audit log", exact: true }).click();
  }

  /** The entries of the audit log. */
  get log(): Locator {
    return this.main.getByRole("list", { name: "Audit log", exact: true });
  }

  /** The entry of the audit log whose sentence holds `sentence`. */
  entry(sentence: string): Locator {
    return this.log.getByRole("listitem").filter({ hasText: sentence });
  }
}

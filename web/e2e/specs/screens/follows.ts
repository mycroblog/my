// SPDX-License-Identifier: AGPL-3.0-only

// A person's lists of follows and followers (/@username/following, /@username/followers): one screen whose two tabs
// each hold a list, a row per account with a link to its profile and its follow button; and the counts of the profile
// that open them.

import type { Locator, Page } from "@playwright/test";
import type { Person } from "../../src/scenario";
import { answered } from "./write";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

/** Which list: whom the person follows, or who follows them. */
export type FollowsList = "Following" | "Followers";

/** The link of a profile's card that opens its list `list`, named by the list and its count ("Followers 12"). */
export function countLink(card: Locator, list: FollowsList): Locator {
  return card.getByRole("link", { name: new RegExp(`^${list} [\\d,]+$`) });
}

export class Follows {
  readonly page: Page;
  readonly person: Person;

  constructor(page: Page, person: Person) {
    this.page = page;
    this.person = person;
  }

  /** The address of the list `list`. */
  address(list: FollowsList): RegExp {
    return new RegExp(`/@${escape(this.person.username)}/${list.toLowerCase()}$`);
  }

  /** The tab of the list `list`, named by the list and its count. */
  tab(list: FollowsList): Locator {
    return this.page
      .getByRole("main")
      .getByRole("tablist", { name: "Following and followers", exact: true })
      .getByRole("tab", { name: new RegExp(`^${list} [\\d,]+$`) });
  }

  /** The open list. */
  get panel(): Locator {
    return this.page.getByRole("main").getByRole("tabpanel");
  }

  /** The link of the open list's row of `p` to their profile, named by their display name. */
  row(p: Person): Locator {
    return this.panel.getByRole("link", { name: p.displayName, exact: true });
  }

  /** The follow buttons of the open list's rows: of the accounts the reader does not follow. */
  get followButtons(): Locator {
    return this.panel.getByRole("button", { name: /^Follow( back)?$/ });
  }

  /** Follows the account of the one row that offers a follow, and returns once the server has answered. */
  async followTheOne(): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/accounts\/[^/]+\/follow$/ }, async () => {
      await this.followButtons.click();
    });
  }
}

// SPDX-License-Identifier: AGPL-3.0-only

// The profile of an account that moved (ADR-0089 §3, §4): its main action opens the new account, and a line names the
// new account with a link to it.

import type { Locator } from "@playwright/test";
import type { Person } from "../../src/scenario";

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

export class Moved {
  readonly card: Locator;
  readonly to: Person;

  /** The profile whose card is `card` (Profile.card), of an account that moved to `to`'s. */
  constructor(card: Locator, to: Person) {
    this.card = card;
    this.to = to;
  }

  /** The main action: the link to the new account. */
  get goToNew(): Locator {
    return this.card.getByRole("link", { name: "Go to new account", exact: true });
  }

  /** The line that names the new account, "Moved to @username@host". */
  get line(): Locator {
    return this.card
      .getByRole("paragraph")
      .filter({ hasText: new RegExp(`^Moved to ⁨?@${escape(this.to.username)}@[^\\s⁩]+⁩?$`) });
  }

  /** The line's link to the new account, named by its handle. */
  get link(): Locator {
    return this.line.getByRole("link", { name: new RegExp(`^@${escape(this.to.username)}@[^\\s]+$`) });
  }

  /** Any follow of the profile's card, which a moved account offers none of. */
  get follows(): Locator {
    return this.card.getByRole("button", { name: /^(Follow|Follow back|Request to follow|Following)$/ });
  }
}

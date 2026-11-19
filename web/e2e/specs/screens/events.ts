// SPDX-License-Identifier: AGPL-3.0-only

// An event's page: the reader's answer to it and to the dates of its poll, each saved at once (the action returns once
// the server answered), who is going, and the calendar file it downloads.

import type { Download, Locator, Page } from "@playwright/test";
import { answered } from "./write";

export class EventPage {
  readonly page: Page;
  readonly id: string;

  constructor(page: Page, id: string) {
    this.page = page;
    this.id = id;
  }

  async open(): Promise<void> {
    await this.page.goto(`/events/${this.id}`);
  }

  /** The event's title, the heading of its page's own head. */
  title(title: string): Locator {
    return this.page.getByRole("main").getByRole("heading", { level: 2, name: title, exact: true });
  }

  /** The reader's answers to the event, a radio group. */
  get answers(): Locator {
    return this.page.getByRole("main").getByRole("radiogroup", { name: "Your answer", exact: true });
  }

  /** Answers the event `answer` (Going, Maybe, Not going), and returns once the server saved it. */
  async answer(answer: "Going" | "Maybe" | "Not going"): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/api\/v1\/events\/[^/]+\/rsvp$/ }, () =>
      this.answers.getByRole("radio", { name: answer, exact: true }).click(),
    );
  }

  /** The list of who answered going, named by its heading with how many. */
  going(count: number): Locator {
    return this.page.getByRole("main").getByRole("list", { name: `Going (${String(count)})`, exact: true });
  }

  /** Downloads the event's calendar file by Add to calendar. */
  async downloadCalendar(): Promise<Download> {
    const download = this.page.waitForEvent("download");
    await this.page.getByRole("main").getByRole("button", { name: "Add to calendar", exact: true }).click();
    return download;
  }

  /** The table of the poll's dates. */
  get poll(): Locator {
    return this.page.getByRole("main").getByRole("table", { name: "Date poll", exact: true });
  }

  /**
   * The reader's answers to the poll's date that starts at `start`, a radio group named by its date, as the browser's
   * zone shows it ("Mon, Nov 23, 2:00 – 4:00 PM"), and its column ("Your answer"): the specs and the browser share a
   * zone, so the day there is the day the row shows.
   */
  dateAnswers(start: Date): Locator {
    const day = new Intl.DateTimeFormat("en", { month: "short", day: "numeric" }).format(start);
    return this.poll.getByRole("radiogroup", { name: new RegExp(`^\\w+, ${day}, .* Your answer$`) });
  }

  /** Answers the poll's date that starts at `start` `vote` (Can go, Maybe, Cannot go), once the server saved it. */
  async answerDate(start: Date, vote: "Can go" | "Maybe" | "Cannot go"): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/api\/v1\/events\/[^/]+\/candidates\/[^/]+\/vote$/ }, () =>
      this.dateAnswers(start).getByRole("radio", { name: vote, exact: true }).click(),
    );
  }
}

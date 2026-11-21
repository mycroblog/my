// SPDX-License-Identifier: AGPL-3.0-only

// An event's page: the reader's answer to it and to the dates of its poll, each saved at once (the action returns once
// the server answered), who is going, the calendar file it downloads, and its board under the Board tab.

import { expect, type Download, type Locator, type Page } from "@playwright/test";
import { answered } from "./write";

/** The form that creates an event, reached from the list's Create an event. */
export class EventForm {
  readonly page: Page;

  constructor(page: Page) {
    this.page = page;
  }

  /** Opens the list of events and follows its Create an event. */
  async openFromList(): Promise<void> {
    await this.page.goto("/events");
    await this.page.getByRole("link", { name: "Create an event", exact: true }).click();
    await expect(this.title).toBeVisible();
  }

  get title(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Title", exact: true });
  }

  /** The choice of polling the date in place of a fixed one. */
  get pollDates(): Locator {
    return this.page.getByRole("main").getByRole("radio", { name: /^Poll for a date: .+$/ });
  }

  /** Chooses to poll the date. */
  async choosePoll(): Promise<void> {
    await this.pollDates.click();
    await expect(this.pollDates).toBeChecked();
  }

  get voice(): Locator {
    return this.page
      .getByRole("main")
      .getByRole("checkbox", { name: "In a voice chat room that opens when the event starts", exact: true });
  }

  get inPerson(): Locator {
    return this.page.getByRole("main").getByRole("checkbox", { name: "In person", exact: true });
  }

  get placeName(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Name", exact: true });
  }

  get address(): Locator {
    return this.page.getByRole("main").getByRole("textbox", { name: "Address", exact: true });
  }

  get submit(): Locator {
    return this.page.getByRole("main").getByRole("button", { name: "Create the event", exact: true });
  }

  /**
   * Sets the poll's first dates the form offers to start at `starts`, two hours long each, as the browser's zone reads
   * them (the specs and the browser share a zone).
   */
  async pollDatesAt(starts: readonly Date[]): Promise<void> {
    const p = (v: number) => String(v).padStart(2, "0");
    const local = (d: Date) =>
      `${String(d.getFullYear())}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
    for (const [i, start] of starts.entries()) {
      const group = this.page.getByRole("main").getByRole("group", { name: `Date ${String(i + 1)}`, exact: true });
      await group.getByLabel("Starts", { exact: true }).fill(local(start));
      await group.getByLabel("Ends", { exact: true }).fill(local(new Date(start.getTime() + 2 * 3_600_000)));
    }
  }
}

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

  /**
   * Confirms the poll's date that starts at `start` from the menu at the end of its row, asked first, once the server
   * confirmed it.
   */
  async confirmDate(start: Date): Promise<void> {
    // Its menu is named by its date, as the browser's zone shows it ("Options for Mon, Nov 23, 6:00 – 8:00 PM").
    const day = new Intl.DateTimeFormat("en", { month: "short", day: "numeric" }).format(start);
    await this.poll.getByRole("button", { name: new RegExp(`^Options for \\w+, ${day}, .+$`) }).click();
    await this.page.getByRole("menuitem", { name: "Confirm this date…", exact: true }).click();
    await answered(
      this.page,
      { methods: ["POST"], path: /\/api\/v1\/events\/[^/]+\/candidates\/[^/]+\/confirm$/ },
      () => this.page.getByRole("alertdialog").getByRole("button", { name: "Confirm", exact: true }).click(),
    );
  }

  /** Approves the attendee of `name`, waiting for approval, by the Approve of their row, once the server approved them. */
  async approve(name: string): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/api\/v1\/events\/[^/]+\/attendees\/[^/]+\/approval$/ }, () =>
      this.page
        .getByRole("main")
        .getByRole("button", { name: `Approve ${name}`, exact: true })
        .click(),
    );
  }

  /** Cancels the event from its menu with `note`, asked first, once the server cancelled it. */
  async cancel(note: string): Promise<void> {
    await this.page.getByRole("main").getByRole("button", { name: "Manage the event", exact: true }).click();
    await this.page.getByRole("menuitem", { name: "Cancel the event…", exact: true }).click();
    const dialog = this.page.getByRole("dialog", { name: "Cancel the event?", exact: true });
    await dialog.getByRole("textbox").fill(note);
    await answered(this.page, { methods: ["POST"], path: /\/api\/v1\/events\/[^/]+\/cancel$/ }, () =>
      dialog.getByRole("button", { name: "Cancel the event", exact: true }).click(),
    );
  }

  /** The event's tabs, Details and Board, named by its title: there for a reader of the board alone. */
  tabs(title: string): Locator {
    return this.page.getByRole("main").getByRole("tablist", { name: title, exact: true });
  }

  /** The tab `name` of the event titled `title`. */
  tab(title: string, name: "Details" | "Board"): Locator {
    return this.tabs(title).getByRole("tab", { name, exact: true });
  }

  /** The posts of the board, newest first, in the Board tab. */
  get board(): Locator {
    return this.page.getByRole("main").getByRole("feed", { name: "The event's board", exact: true });
  }

  /** The post on the board that holds `text`. */
  boardPost(text: string): Locator {
    return this.board.getByRole("article").filter({ hasText: text });
  }

  /** Opens the composer of a post on the board by the bar of the Board tab. */
  async writeOnTheBoard(): Promise<Locator> {
    await this.page.getByRole("main").getByRole("button", { name: "Write on the board", exact: true }).click();
    return this.page.getByRole("dialog", { name: "New post", exact: true });
  }

  /** Posts what `composer`, the board's, holds, and returns once the server has answered and the composer has closed. */
  async postOnTheBoard(composer: Locator): Promise<void> {
    await answered(this.page, { methods: ["POST"], path: /\/api\/v1\/events\/[^/]+\/board$/ }, () =>
      composer.getByRole("button", { name: "Post", exact: true }).click(),
    );
    await expect(composer).toBeHidden();
  }

  /** Answers the poll's date that starts at `start` `vote` (Can go, Maybe, Cannot go), once the server saved it. */
  async answerDate(start: Date, vote: "Can go" | "Maybe" | "Cannot go"): Promise<void> {
    await answered(this.page, { methods: ["PUT"], path: /\/api\/v1\/events\/[^/]+\/candidates\/[^/]+\/vote$/ }, () =>
      this.dateAnswers(start).getByRole("radio", { name: vote, exact: true }).click(),
    );
  }
}

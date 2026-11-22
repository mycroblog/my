// SPDX-License-Identifier: AGPL-3.0-only

// The parts of an event (#3236), as assistive technology gets them: a time with the event's own beside the reader's;
// an event's line, one link named by all it shows; the reader's answer, a radio group a press and the arrow keys
// change, its note describing it; the date poll, a table whose first row names the columns and whose every other row
// is headed by its date, its cells in the columns' order, its answer a radio group named by the date and the column;
// and the places, a link that leaves the site saying where and that it opens in a new tab, a hidden place its kind
// alone. How the table stands at a width is a story's (Events).

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  AnswerChoice,
  CandidateRow,
  CandidateTable,
  EventLine,
  EventTime,
  PlaceLine,
  PlaceList,
  type Answer,
} from "./event";

const going = { yes: "Going", maybe: "Maybe", no: "Not going" } as const;
const poll = { yes: "Can go", maybe: "Maybe", no: "Cannot" } as const;

test("an event's time is a <time> in the reader's zone, then the event's own with its zone's place name", () => {
  const { container } = render(
    <EventTime
      dateTime="2026-11-21T05:00:00Z"
      shown="Fri, Nov 20, 9:00 PM"
      own={{ shown: "Sat 2:00 PM", zone: "Tokyo" }}
    />,
  );

  const time = container.querySelector("time");
  expect(time?.getAttribute("datetime")).toBe("2026-11-21T05:00:00Z");
  expect(time?.textContent).toBe("Fri, Nov 20, 9:00 PM");
  expect(container.textContent).toBe("Fri, Nov 20, 9:00 PMSat 2:00 PM (Tokyo)");
});

test("an event's time where the zones agree shows the reader's alone", () => {
  const { container } = render(<EventTime dateTime="2026-11-24T03:00:00Z" shown="Mon, Nov 23, 7:00 PM" />);

  expect(container.textContent).toBe("Mon, Nov 23, 7:00 PM");
});

test("an event's line is one link to it, named by its date, its title, its state and time, its places and who", () => {
  render(
    <EventLine
      href="/events/1"
      title="Morning walk"
      date={{ month: "Nov", day: "28" }}
      time="Sat, Nov 28, 5:00 PM"
      places={[{ kind: "physical", label: "In person" }]}
      organizer="By Aoi"
      going={{ shown: "4", label: "4 going" }}
      state={{ kind: "canceled", label: "Cancelled" }}
    />,
  );

  const link = screen.getByRole("link");
  expect(link.getAttribute("href")).toBe("/events/1");
  expect(link.textContent).toBe("Nov 28Morning walkCancelledSat, Nov 28, 5:00 PMIn personBy Aoi44 going");
});

test("an event's line says an ended event and one whose date is polled, each still one link", () => {
  const { unmount } = render(
    <EventLine
      href="/events/6"
      title="Sketch night"
      date={{ month: "Nov", day: "9" }}
      time="Mon, Nov 9, 7:00 PM"
      organizer="By Kai"
      going={{ shown: "7", label: "7 went" }}
      state={{ kind: "ended", label: "Ended" }}
    />,
  );
  expect(screen.getAllByRole("link")).toHaveLength(1);
  expect(screen.getByRole("link").textContent).toBe("Nov 9Sketch nightEndedMon, Nov 9, 7:00 PMBy Kai77 went");
  unmount();

  render(<EventLine href="/events/3" title="Winter meetup" time="Date to be decided · 4 dates proposed" />);
  expect(screen.getAllByRole("link")).toHaveLength(1);
  expect(screen.getByRole("link").textContent).toBe("Winter meetupDate to be decided · 4 dates proposed");
});

test("the reader's answer is a radio group of three that a press and the arrow keys change", async () => {
  const user = userEvent.setup();
  const chosen: Answer[] = [];
  render(<AnswerChoice label="Your answer" names={going} value="yes" onChange={(a) => chosen.push(a)} />);

  // The choice carries no words of its own beside its answers: with no reason given, nothing describes it.
  const group = screen.getByRole("radiogroup", { name: "Your answer", description: "" });
  expect(group.textContent).toBe("GoingMaybeNot going");
  const radios = within(group).getAllByRole("radio");
  expect(radios.map((r) => r.getAttribute("aria-label") ?? r.closest("label")?.textContent)).toEqual([
    "Going",
    "Maybe",
    "Not going",
  ]);
  expect(screen.getByRole("radio", { name: "Going" })).toHaveProperty("checked", true);

  await user.click(screen.getByRole("radio", { name: "Not going" }));
  expect(chosen).toEqual(["no"]);
});

test("an answer that cannot be chosen now (going, at capacity) is disabled, the others are not", () => {
  render(<AnswerChoice label="Your answer" names={going} value={null} onChange={() => undefined} disabled={["yes"]} />);

  expect(screen.getByRole("radio", { name: "Going" })).toHaveProperty("disabled", true);
  expect(screen.getByRole("radio", { name: "Maybe" })).toHaveProperty("disabled", false);
});

test("no answer may be given while the whole choice is disabled", () => {
  render(<AnswerChoice label="Your answer" names={going} value="maybe" onChange={() => undefined} isDisabled />);

  for (const radio of screen.getAllByRole("radio")) expect(radio).toHaveProperty("disabled", true);
});

test("the save state stands in a status region there from the start", () => {
  const { rerender } = render(
    <AnswerChoice label="Your answer" names={going} value="yes" onChange={() => undefined} save={null} />,
  );
  const status = screen.getByRole("status");
  expect(status.textContent).toBe("");

  rerender(<AnswerChoice label="Your answer" names={going} value="yes" onChange={() => undefined} save="Saved" />);
  expect(screen.getByRole("status").textContent).toBe("Saved");
});

test("an answer that cannot be chosen now is passed by the arrow keys, and a press on it chooses nothing", async () => {
  const user = userEvent.setup();
  const chosen: Answer[] = [];
  render(
    <AnswerChoice
      label="Your answer"
      names={going}
      value={null}
      onChange={(a) => chosen.push(a)}
      disabled={["yes"]}
      note="The event is full: no one more can go."
    />,
  );

  await user.tab();
  expect(document.activeElement === screen.getByRole("radio", { name: "Maybe" })).toBe(true);
  await user.keyboard("{ArrowRight}");
  await user.keyboard("{ArrowRight}");
  await user.click(screen.getByRole("radio", { name: "Going" }));

  expect(chosen).toEqual(["no", "maybe"]);
  expect(screen.getByRole("radiogroup", { name: "Your answer" }).getAttribute("aria-describedby")).not.toBeNull();
  expect(screen.getAllByRole("radiogroup", { description: "The event is full: no one more can go." })).toHaveLength(1);
});

test("while the save state shows in the reason's place, the reason still describes the group", () => {
  render(
    <AnswerChoice
      label="Your answer"
      names={going}
      value="maybe"
      onChange={() => undefined}
      disabled={["yes"]}
      note="The event is full: no one more can go."
      save="Saving…"
    />,
  );

  expect(
    screen.getAllByRole("radiogroup", {
      name: "Your answer",
      description: "The event is full: no one more can go.",
    }),
  ).toHaveLength(1);
  expect(screen.getByRole("status").textContent).toBe("Saving…");
});

/** The date poll of two dates, the reader's answers told to `answered`, with the organizers' actions or not. */
function table(answered: (row: number, a: Answer) => void, actions = false) {
  render(
    <CandidateTable
      label="Date poll"
      time="Date"
      names={poll}
      yours="Your answer"
      actions={actions ? "Actions" : undefined}
    >
      <CandidateRow
        time="Sat, Dec 5, 1:00 PM"
        counts={{ yes: "5", maybe: "2", no: "1" }}
        answer="yes"
        onAnswer={(a) => {
          answered(0, a);
        }}
        confirmed="Confirmed"
        actions={actions ? <button type="button">Options</button> : undefined}
      />
      <CandidateRow
        time="Sun, Dec 6, 1:00 PM"
        counts={{ yes: "3", maybe: "4", no: "0" }}
        answer={null}
        onAnswer={(a) => {
          answered(1, a);
        }}
        actions={actions ? <button type="button">Options</button> : undefined}
      />
    </CandidateTable>,
  );
  return within(screen.getByRole("table", { name: "Date poll" }));
}

test("the date poll is a table: its first row names the columns, every other row is headed by its date", () => {
  const t = table(() => undefined);

  const rows = t.getAllByRole("row");
  expect(rows).toHaveLength(3);
  const [head, first, second] = rows;
  if (head === undefined || first === undefined || second === undefined) throw new Error("rows");
  expect(
    within(head)
      .getAllByRole("columnheader")
      .map((h) => h.textContent),
  ).toEqual(["Date", "Can go", "Maybe", "Cannot", "Your answer"]);
  expect(within(first).getByRole("rowheader").textContent).toBe("Sat, Dec 5, 1:00 PMConfirmed");
  expect(within(second).getByRole("rowheader").textContent).toBe("Sun, Dec 6, 1:00 PM");
  // The cells in the columns' order: the counts, then the reader's answer.
  expect(
    within(second)
      .getAllByRole("cell")
      .map((c) => c.textContent),
  ).toEqual(["3", "4", "0", "Can goMaybeCannot"]);
});

test("a date's answer is a radio group named by its date and the column, which a press answers", async () => {
  const user = userEvent.setup();
  const answered: [number, Answer][] = [];
  const t = table((row, a) => answered.push([row, a]));

  const groups = t.getAllByRole("radiogroup");
  const [first, second] = groups;
  if (first === undefined || second === undefined) throw new Error("groups");
  const names = (group: HTMLElement) =>
    (group.getAttribute("aria-labelledby") ?? "")
      .split(" ")
      .map((id) => document.getElementById(id)?.textContent)
      .join(" ");
  expect(names(second)).toBe("Sun, Dec 6, 1:00 PM Your answer");
  expect(within(first).getByRole("radio", { name: "Can go" })).toHaveProperty("checked", true);

  await user.click(within(second).getByRole("radio", { name: "Maybe" }));
  expect(answered).toEqual([[1, "maybe"]]);
});

test("for the organizers a column of each date's actions ends the table", () => {
  const t = table(() => undefined, true);

  const [head, first] = t.getAllByRole("row");
  if (head === undefined || first === undefined) throw new Error("rows");
  expect(within(head).getAllByRole("columnheader").at(-1)?.textContent).toBe("Actions");
  expect(
    within(within(first).getAllByRole("cell").at(-1) ?? first).getByRole("button", { name: "Options" }),
  ).toBeTruthy();
});

/** A cell or a header as it is read: its words, the spaces between them folded. */
const words = (element: HTMLElement) => element.textContent.replace(/\s+/g, " ").trim();

/**
 * The poll's every cell as a screen reader reads it on its way through the table, cell by cell: the header of its row
 * (its date), the header of its column (counted by its place in the row), then the cell; a row whose cells do not
 * line up with the columns' headers is told as such.
 */
function heard(table: HTMLElement): string[] {
  const [head, ...rows] = within(table).getAllByRole("row");
  if (head === undefined) return ["no row"];
  const columns = within(head).getAllByRole("columnheader").map(words);
  return rows.flatMap((row) => {
    // The row's headers and cells in the order they are read, which is the document's.
    const read = [...within(row).queryAllByRole("rowheader"), ...within(row).queryAllByRole("cell")].sort((a, b) =>
      a.compareDocumentPosition(b) & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : 1,
    );
    const [date] = within(row).getAllByRole("rowheader");
    if (read.length !== columns.length || date === undefined || read[0] !== date)
      return [`a row of ${read.length} cells under ${columns.length} columns, headed by ${words(date ?? row)}`];
    return read.slice(1).map((cell, i) => `${words(date)} | ${columns[i + 1] ?? ""} | ${words(cell)}`);
  });
}

test("the date poll is read cell by cell, each cell with its date and its column's name", () => {
  render(
    <CandidateTable label="Date poll" time="Date" names={poll} yours="Your answer" actions="Actions" save={null}>
      <CandidateRow
        time={
          <EventTime
            dateTime="2026-12-05T09:00:00Z"
            shown="Sat, Dec 5, 1:00 PM"
            own={{ shown: "Sat 6:00 PM", zone: "Tokyo" }}
          />
        }
        counts={{ yes: "5", maybe: "2", no: "1" }}
        answer="yes"
        onAnswer={() => undefined}
        confirmed="Confirmed"
        actions={<button type="button">Options for Sat, Dec 5</button>}
      />
      <CandidateRow
        time={<EventTime dateTime="2026-12-13T02:00:00Z" shown="Sat, Dec 12, 6:00 PM" />}
        counts={{ yes: "0", maybe: "0", no: "0" }}
        answer={null}
        onAnswer={() => undefined}
        actions={<button type="button">Options for Sat, Dec 12</button>}
      />
    </CandidateTable>,
  );
  const table = screen.getByRole("table", { name: "Date poll" });

  const first = "Sat, Dec 5, 1:00 PMSat 6:00 PM (Tokyo)Confirmed";
  const second = "Sat, Dec 12, 6:00 PM";
  expect(heard(table)).toEqual([
    `${first} | Can go | 5`,
    `${first} | Maybe | 2`,
    `${first} | Cannot | 1`,
    `${first} | Your answer | Can goMaybeCannot`,
    `${first} | Actions | Options for Sat, Dec 5`,
    `${second} | Can go | 0`,
    `${second} | Maybe | 0`,
    `${second} | Cannot | 0`,
    `${second} | Your answer | Can goMaybeCannot`,
    `${second} | Actions | Options for Sat, Dec 12`,
  ]);
  // The answer's group, met in a cell, is named as the cell is read: its date, then its column.
  const yours = within(table).getAllByRole("radiogroup");
  expect(yours).toHaveLength(2);
  expect(within(table).getAllByRole("radiogroup", { name: /^Sat, Dec 5, 1:00 PM.*Your answer$/ })).toHaveLength(1);
  expect(within(table).getAllByRole("radiogroup", { name: /^Sat, Dec 12, 6:00 PM\s*Your answer$/ })).toHaveLength(1);
});

test("the poll's save state stands in a status region outside the table, there from the start", () => {
  const poll2 = (save: string | null) => (
    <CandidateTable label="Date poll" time="Date" names={poll} yours="Your answer" save={save}>
      <CandidateRow
        time="Sat, Dec 5, 1:00 PM"
        counts={{ yes: "5", maybe: "2", no: "1" }}
        answer="yes"
        onAnswer={() => undefined}
      />
    </CandidateTable>
  );
  const { rerender } = render(poll2(null));
  expect(screen.getAllByRole("status").map(words)).toEqual([""]);
  expect(within(screen.getByRole("table", { name: "Date poll" })).queryAllByRole("status")).toHaveLength(0);

  rerender(poll2("Saved"));
  expect(screen.getAllByRole("status").map(words)).toEqual(["Saved"]);
});

test("once the poll is closed each date's answer is held: its radios cannot be chosen, the reader's stays", async () => {
  const user = userEvent.setup();
  const answered: Answer[] = [];
  render(
    <CandidateTable label="Date poll" time="Date" names={poll} yours="Your answer">
      <CandidateRow
        time="Sat, Dec 5, 1:00 PM"
        counts={{ yes: "5", maybe: "2", no: "1" }}
        answer="maybe"
        onAnswer={(a) => answered.push(a)}
        isDisabled
        confirmed="Confirmed"
      />
    </CandidateTable>,
  );

  const radios = screen.getAllByRole("radio");
  expect(
    radios.map((r) => [r.getAttribute("aria-label") ?? words(r.closest("label") ?? r), r.matches(":disabled")]),
  ).toEqual([
    ["Can go", true],
    ["Maybe", true],
    ["Cannot", true],
  ]);
  expect(screen.getByRole("radio", { name: "Maybe" })).toHaveProperty("checked", true);
  await user.click(screen.getByRole("radio", { name: "Can go" }));
  expect(answered).toEqual([]);
});

test("a place's link that leaves the site opens in a new tab, says so and where, and tells nothing of this one", () => {
  render(
    <PlaceList label="Where">
      <PlaceLine
        kind="physical"
        label="In person"
        name="Community hall"
        address={"1-2-3 Jingumae\nShibuya"}
        link={{ href: "https://maps.example.com/?q=hall", text: "Open the map", host: "maps.example.com" }}
      />
      <PlaceLine
        kind="url"
        label="Online"
        name="The stream"
        link={{ href: "https://video.example.org/live", text: "", host: "video.example.org" }}
      />
    </PlaceList>,
  );

  const list = screen.getByRole("list", { name: "Where" });
  expect(within(list).getAllByRole("listitem")).toHaveLength(2);
  const map = screen.getByRole("link", { name: "Open the map (opens maps.example.com in a new tab)" });
  expect(map.getAttribute("target")).toBe("_blank");
  expect(map.getAttribute("rel")).toBe("nofollow noopener noreferrer");
  const stream = screen.getByRole("link", { name: "The stream (opens video.example.org in a new tab)" });
  expect(stream.getAttribute("href")).toBe("https://video.example.org/live");
  // The kind is said before the name.
  expect(screen.getAllByRole("listitem")[0]?.textContent).toStartWith("In person: Community hall");
});

test("a place the reader may not see shows its kind alone: no name, no address, no link", () => {
  render(
    <PlaceList label="Where">
      <PlaceLine
        kind="physical"
        label="In person"
        name="Community hall, room 3"
        address={"1-2-3 Jingumae\nShibuya, Tokyo 150-0001"}
        link={{ href: "https://maps.example.com/?q=hall", text: "Open the map", host: "maps.example.com" }}
        hidden
      />
      <PlaceLine
        kind="url"
        label="Online"
        name="The stream"
        link={{ href: "https://video.example.org/live", text: "", host: "video.example.org" }}
        hidden
      />
    </PlaceList>,
  );

  expect(screen.queryAllByRole("link")).toHaveLength(0);
  expect(screen.getAllByRole("listitem").map((item) => item.textContent)).toEqual(["In person", "Online"]);
});

test("a voice room that could not open says why, and offers no way in", () => {
  render(
    <PlaceList label="Where">
      <PlaceLine
        kind="voice"
        label="Voice chat room"
        unavailable
        note="Could not open: the organizer was hosting another room"
      />
    </PlaceList>,
  );

  expect(screen.getByRole("listitem").textContent).toBe(
    "Voice chat roomCould not open: the organizer was hosting another room",
  );
  expect(screen.queryAllByRole("link")).toHaveLength(0);
  expect(screen.queryAllByRole("button")).toHaveLength(0);
});

test("the voice room shows its state, and the way in while it is open", () => {
  render(
    <PlaceList label="Where">
      <PlaceLine kind="voice" label="Voice chat room" note="Open now" action={<a href="/voice/1">Join the room</a>} />
    </PlaceList>,
  );

  expect(screen.getByRole("listitem").textContent).toBe("Voice chat roomOpen nowJoin the room");
  expect(screen.getByRole("link", { name: "Join the room" }).getAttribute("href")).toBe("/voice/1");
});

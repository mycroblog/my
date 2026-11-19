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
      state={{ kind: "cancelled", label: "Cancelled" }}
    />,
  );

  const link = screen.getByRole("link");
  expect(link.getAttribute("href")).toBe("/events/1");
  expect(link.textContent).toBe("Nov 28Morning walkCancelledSat, Nov 28, 5:00 PMIn personBy Aoi44 going");
});

test("the reader's answer is a radio group of three that a press and the arrow keys change", async () => {
  const user = userEvent.setup();
  const chosen: Answer[] = [];
  render(
    <AnswerChoice
      label="Your answer"
      names={going}
      value="yes"
      onChange={(a) => chosen.push(a)}
      note="Your name shows in the list of who answered."
    />,
  );

  const group = screen.getByRole("radiogroup", { name: "Your answer" });
  expect(group.getAttribute("aria-describedby")).not.toBeNull();
  expect(document.getElementById(group.getAttribute("aria-describedby") ?? "")?.textContent).toBe(
    "Your name shows in the list of who answered.",
  );
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

test("a place the reader may not see shows its kind alone, and why", () => {
  render(
    <PlaceList label="Where">
      <PlaceLine
        kind="url"
        label="Online"
        name="The stream"
        link={{ href: "https://video.example.org/live", text: "", host: "video.example.org" }}
        hidden
        note="Shown to approved participants"
      />
    </PlaceList>,
  );

  expect(screen.queryAllByRole("link")).toHaveLength(0);
  expect(screen.getByRole("listitem").textContent).toBe("OnlineShown to approved participants");
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

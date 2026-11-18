// SPDX-License-Identifier: AGPL-3.0-only

// What the drafts' list and the schedule form are to a reader and to assistive technology: the sections' headings, a
// row that opens its draft and says what it shows, the time field with its limits and its error, and what a press
// asks of the caller.

import { expect, test } from "bun:test";
import { I18nProvider, IconButton, TimeZoneProvider } from "@my/design-system";
import { fireEvent, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import {
  DraftCount,
  DraftList,
  DraftRow,
  DraftSection,
  DraftSections,
  DraftsEmpty,
  localInput,
  readerZone,
  ScheduleForm,
  ScheduleTime,
  ScheduleZone,
  scheduleTime,
  type DraftRowProps,
  type ScheduleFormProps,
} from "./drafts";

const none = () => undefined;

const draft: DraftRowProps = {
  text: "Bread at ten on Saturday",
  blank: "No text",
  marks: [],
  saved: "Saved 2h ago",
  openLabel: "Open this draft",
  onOpen: none,
  menu: <IconButton icon="more" label="More" />,
};

test("the sections of the drafts are headed, each heading with how many it holds, over a named list of its rows", () => {
  render(
    <DraftSections>
      <DraftSection
        title={
          <>
            Scheduled <DraftCount count={2} />
          </>
        }
        note={<span>Asia/Tokyo · GMT+9</span>}
      >
        <DraftList label="Scheduled posts">
          <DraftRow {...draft} text="First" />
          <DraftRow {...draft} text="Second" />
        </DraftList>
      </DraftSection>
      <DraftSection title="Drafts">
        <DraftsEmpty>No drafts yet.</DraftsEmpty>
      </DraftSection>
    </DraftSections>,
  );

  expect(screen.getByRole("heading", { name: /^Scheduled\s*2$/ })).not.toBeNull();
  expect(screen.getByText("Asia/Tokyo · GMT+9")).not.toBeNull();
  expect(within(screen.getByRole("list", { name: "Scheduled posts" })).getAllByRole("listitem")).toHaveLength(2);
  expect(screen.getByRole("heading", { name: "Drafts" })).not.toBeNull();
  expect(screen.getByText("No drafts yet.")).not.toBeNull();
});

test("a row is a button that says what it does, then what it shows: the warning, the text, what it carries, when it was saved", async () => {
  const user = userEvent.setup();
  let opened = 0;
  render(
    <DraftList label="Drafts">
      <DraftRow
        {...draft}
        warning="Food"
        lang="ja"
        marks={[
          { icon: "attach", label: "2 images", count: 2 },
          { icon: "poll", label: "A poll" },
        ]}
        onOpen={() => opened++}
      />
    </DraftList>,
  );
  const row = screen.getByRole("button", { name: /^Open this draft\s*Food\s*Bread at ten on Saturday/ });

  expect(row.textContent).toContain("Saved 2h ago");
  expect(within(row).getByRole("img", { name: "2 images" })).not.toBeNull();
  expect(within(row).getByRole("img", { name: "A poll" })).not.toBeNull();
  expect(
    within(row)
      .getByText(/Bread at ten on Saturday/)
      .closest("[lang]")
      ?.getAttribute("lang"),
  ).toBe("ja");
  await user.click(row);
  expect(opened).toBe(1);
});

test("Tab reaches a row, then its menu; Enter on the row opens the draft, and a press on the menu does not", async () => {
  const user = userEvent.setup();
  let opened = 0;
  let menu = 0;
  render(
    <DraftList label="Drafts">
      <DraftRow
        {...draft}
        onOpen={() => opened++}
        menu={<IconButton icon="more" label="More" onPress={() => menu++} />}
      />
    </DraftList>,
  );

  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: /^Open this draft/ })).toBe(true);
  await user.keyboard("{Enter}");
  expect(opened).toBe(1);
  await user.tab();
  expect(document.activeElement === screen.getByRole("button", { name: "More" })).toBe(true);
  await user.keyboard("{Enter}");
  await user.click(screen.getByRole("button", { name: "More" }));

  expect(menu).toBe(2);
  expect(opened).toBe(1);
});

test("a draft with no text says so in its place", () => {
  render(
    <DraftList label="Drafts">
      <DraftRow {...draft} text="" />
    </DraftList>,
  );

  expect(screen.getByRole("button", { name: /^Open this draft\s*No text\s*Saved 2h ago$/ })).not.toBeNull();
});

test("a scheduled post leads with the time it goes out, and one that was not posted with why", () => {
  render(
    <DraftList label="Scheduled posts">
      <DraftRow {...draft} lead={{ kind: "scheduled", text: <ScheduleTime date="2026-11-16T09:00:00Z" /> }} />
      <DraftRow {...draft} text="Second" lead={{ kind: "failed", text: "Not posted: an image is no longer there." }} />
    </DraftList>,
  );
  const [scheduled, failed] = screen.getAllByRole("button", { name: /^Open this draft/ });
  if (scheduled === undefined || failed === undefined) throw new Error("the list has no two rows");

  const time = within(scheduled).getByRole<HTMLTimeElement>("time");
  expect(time.dateTime).toBe("2026-11-16T09:00:00.000Z");
  expect(
    time.compareDocumentPosition(within(scheduled).getByText(/Bread at ten/)) & Node.DOCUMENT_POSITION_FOLLOWING,
  ).not.toBe(0);
  const why = within(failed).getByText("Not posted: an image is no longer there.");
  expect(why.compareDocumentPosition(within(failed).getByText("Second")) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(
    0,
  );
});

test("the time a post goes out and the reader's zone read in the reader's locale, each isolated", () => {
  const at = "2026-11-16T09:00:00Z";
  const { unmount } = render(
    <I18nProvider locale="ja">
      <ScheduleTime date={at} /> <ScheduleZone />
    </I18nProvider>,
  );
  const time = screen.getByRole<HTMLTimeElement>("time");
  expect(time.textContent).toBe(scheduleTime(at, "ja"));
  expect(time.closest("bdi")).not.toBeNull();
  expect(screen.getByText(readerZone("ja")).tagName).toBe("BDI");
  unmount();

  render(
    <I18nProvider locale="en">
      <ScheduleTime date={new Date(at)} />
    </I18nProvider>,
  );
  expect(screen.getByRole("time").textContent).toBe(scheduleTime(at, "en"));
});

const min = new Date(2026, 10, 15, 12, 5);
const max = new Date(2027, 10, 16, 12, 0);

/** A field of a date and a time has no role to find it by: the field of the label, and the words that describe it. */
const timeField = () => screen.getByLabelText<HTMLInputElement>("Date and time");
const described = (field: Element) =>
  (field.getAttribute("aria-describedby") ?? "").split(" ").map((id) => document.getElementById(id)?.textContent ?? "");

const form: ScheduleFormProps = {
  title: "Schedule",
  label: "Date and time",
  value: "2026-11-16T09:00",
  onChange: none,
  min,
  max,
  zone: "Asia/Tokyo · GMT+9",
  hint: "From 5 minutes to 366 days from now.",
  confirm: { label: "Set time", onPress: none },
};

test("the schedule form has one field of the date and the time, named by its label, held to the server's limits", () => {
  render(<ScheduleForm {...form} />);
  const field = timeField();

  expect(field.type).toBe("datetime-local");
  expect(field.value).toBe("2026-11-16T09:00");
  expect(field.min).toBe(localInput(min));
  expect(field.max).toBe(localInput(max));
  expect(screen.getByText("Schedule")).not.toBeNull();
});

test("for a reader who chose a zone, the time, the zone's line and the field's limits are that zone's", () => {
  const zone = "America/New_York";
  // 03:30 UTC on Monday is 22:30 on Sunday there, whatever the device's zone.
  const at = "2026-11-16T03:30:00Z";
  render(
    <TimeZoneProvider timeZone={zone}>
      <ScheduleTime date={at} /> <ScheduleZone />
      <ScheduleForm {...form} />
    </TimeZoneProvider>,
  );

  expect(screen.getByRole("time").textContent).toBe("Sun, Nov 15, 10:30 PM");
  expect(screen.getByText(/^America\/New_York · GMT-[45]$/).tagName).toBe("BDI");
  const field = timeField();
  expect(field.min).toBe(localInput(min, zone));
  expect(field.max).toBe(localInput(max, zone));
  // The value is the caller's, a date and a time of that zone already.
  expect(field.value).toBe("2026-11-16T09:00");
});

test("the field is described by the reader's zone and the limits, and tells the caller the time chosen", () => {
  const chosen: string[] = [];
  render(<ScheduleForm {...form} onChange={(value) => chosen.push(value)} />);
  const field = timeField();

  expect(described(field)).toEqual(["Asia/Tokyo · GMT+9", "From 5 minutes to 366 days from now."]);
  expect(field.getAttribute("aria-invalid")).toBeNull();
  fireEvent.change(field, { target: { value: "2026-12-24T18:30" } });

  expect(chosen).toEqual(["2026-12-24T18:30"]);
});

test("what is wrong with the time stands in the limits' place, describes the field, and the field is invalid", () => {
  render(<ScheduleForm {...form} value="" error="Choose a time at least 5 minutes from now." />);

  const field = timeField();
  expect(described(field)).toEqual(["Asia/Tokyo · GMT+9", "Choose a time at least 5 minutes from now."]);
  expect(field.getAttribute("aria-invalid")).toBe("true");
  expect(screen.queryByText("From 5 minutes to 366 days from now.")).toBeNull();
});

test("the form's buttons set the time and, while one is set, take it away: the clear first, the confirmation last", async () => {
  const user = userEvent.setup();
  const pressed: string[] = [];
  const { rerender } = render(
    <ScheduleForm
      {...form}
      confirm={{ label: "Set time", onPress: () => pressed.push("confirm") }}
      clear={{ label: "Post now instead", onPress: () => pressed.push("clear") }}
    />,
  );
  const clear = screen.getByRole("button", { name: "Post now instead" });
  const confirm = screen.getByRole("button", { name: "Set time" });

  expect(clear.compareDocumentPosition(confirm) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  await user.click(confirm);
  await user.click(clear);
  expect(pressed).toEqual(["confirm", "clear"]);

  rerender(<ScheduleForm {...form} />);
  expect(screen.getAllByRole("button").map((button) => button.textContent)).toEqual(["Set time"]);
});

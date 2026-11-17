// SPDX-License-Identifier: AGPL-3.0-only

// What a post's versions are to a reader and to assistive technology: a named list, each version with its label, its
// time and its language, and what a version added or removed, which a screen reader hears in the reader's language.

import { expect, test } from "bun:test";
import { isInaccessible, render, screen, within } from "@testing-library/react";
import { DiffMedia, DiffText, Version, VersionList, type DiffMarks } from "./history";

const marks: DiffMarks = {
  lang: "en",
  added: { start: "Start of addition", end: "End of addition" },
  removed: { start: "Start of removal", end: "End of removal" },
  addedImage: "Added image",
  removedImage: "Removed image",
};

const words = (element: Element | null) => element?.textContent.replace(/\s+/g, " ").trim();

test("the versions are a list named by its label, each with its label and its time, what it holds in its language", () => {
  render(
    <VersionList label="Edit history">
      <Version label="Current" time={<time dateTime="2026-11-14T05:00:00Z">Nov 14, 5:00 AM</time>} lang="ja">
        <p>土曜の十時にパン</p>
      </Version>
      <Version label="Original" time={<time dateTime="2026-11-14T03:00:00Z">Nov 14, 3:00 AM</time>}>
        <p>Bread at nine tomorrow</p>
      </Version>
    </VersionList>,
  );
  const [current, original] = within(screen.getByRole("list", { name: "Edit history" })).getAllByRole("listitem");
  if (current === undefined || original === undefined) throw new Error("the list has no two versions");

  expect(within(current).getByText("Current")).not.toBeNull();
  expect(within(current).getByRole<HTMLTimeElement>("time").dateTime).toBe("2026-11-14T05:00:00Z");
  expect(within(current).getByText("土曜の十時にパン").closest("[lang]")?.getAttribute("lang")).toBe("ja");
  expect(within(original).getByText("Original")).not.toBeNull();
  expect(within(original).getByRole<HTMLTimeElement>("time").dateTime).toBe("2026-11-14T03:00:00Z");
  // The label is the reader's words, outside the version's language.
  expect(within(current).getByText("Current").closest("[lang]")).toBeNull();
});

test("a version's text marks what it removed as a deletion and what it added as an insertion, and leaves the rest", () => {
  render(
    <DiffText
      parts={[
        { text: "Bread at ten ", kind: "same" },
        { text: "tomorrow", kind: "removed" },
        { text: "on Saturday", kind: "added" },
      ]}
      marks={marks}
    />,
  );

  expect(screen.getByRole("deletion").textContent).toContain("tomorrow");
  expect(screen.getByRole("insertion").textContent).toContain("on Saturday");
  const same = screen.getByText("Bread at ten");
  expect(same.closest("ins, del")).toBeNull();
});

test("a screen reader hears where each addition and removal starts and ends, in the reader's language", () => {
  render(
    <div lang="ja">
      <DiffText
        parts={[
          { text: "明日", kind: "removed" },
          { text: "土曜", kind: "added" },
        ]}
        marks={marks}
      />
    </div>,
  );

  expect(words(screen.getByRole("deletion"))).toBe("Start of removal 明日 End of removal");
  expect(words(screen.getByRole("insertion"))).toBe("Start of addition 土曜 End of addition");
  for (const said of ["Start of removal", "End of removal", "Start of addition", "End of addition"]) {
    const mark = screen.getByText(said);
    expect(isInaccessible(mark)).toBe(false);
    expect(mark.closest("[lang]")?.getAttribute("lang")).toBe("en");
  }
});

test("a run's text is drawn as its caller draws it", () => {
  render(
    <DiffText
      parts={[
        { text: "Tea :teacup:", kind: "same" },
        { text: ":wave:", kind: "added" },
      ]}
      marks={marks}
      render={(text) => <span data-testid="run">{text.replaceAll(/:(\w+):/g, "[$1]")}</span>}
    />,
  );

  expect(screen.getAllByTestId("run").map((run) => run.textContent)).toEqual(["Tea [teacup]", "[wave]"]);
  expect(within(screen.getByRole("insertion")).getByText("[wave]")).not.toBeNull();
});

test("a version's images are a list, each with its description; an added or removed one says so before it", () => {
  render(
    <DiffMedia
      marks={marks}
      items={[
        { id: "a", src: "https://m.example/snow.webp", alt: "Snow on the ridge", kind: "removed" },
        { id: "b", src: "https://m.example/dusk.webp", alt: "The ridge at dusk", kind: "added" },
        { id: "c", src: "https://m.example/noon.webp", alt: "The ridge at noon", kind: "same" },
      ]}
    />,
  );
  const [removed, added, kept] = within(screen.getByRole("list")).getAllByRole("listitem");
  if (removed === undefined || added === undefined || kept === undefined)
    throw new Error("the list has no three images");

  const snow = within(within(removed).getByRole("deletion")).getByRole("img", { name: "Snow on the ridge" });
  expect(snow.getAttribute("src")).toBe("https://m.example/snow.webp");
  const said = within(removed).getByText("Removed image");
  expect(said.compareDocumentPosition(snow) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(said.closest("[lang]")?.getAttribute("lang")).toBe("en");
  expect(within(within(added).getByRole("insertion")).getByRole("img", { name: "The ridge at dusk" })).not.toBeNull();
  expect(within(added).getByText("Added image")).not.toBeNull();
  expect(within(kept).getByRole("img", { name: "The ridge at noon" }).closest("ins, del")).toBeNull();
  expect(within(kept).queryByText(/image/)).toBeNull();
});

test("a version with no image draws no list", () => {
  const { container } = render(<DiffMedia marks={marks} items={[]} />);

  expect(container.childElementCount).toBe(0);
});

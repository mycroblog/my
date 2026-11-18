// SPDX-License-Identifier: AGPL-3.0-only

// The head of a page with a picture (#3200), a profile's: what it is to assistive technology. On a screen of its own
// it is a region named by the element it is told of; small, inside a section of another screen (the strip of the
// settings' Profile), it is a group of that name and no landmark of its own. Either way the banner is a decoration,
// and the lead, what the reader may do and the rest of the head are inside it. Where the lead stands over the banner
// and how wide the banner is at each width is the story's (Banner head) and the layout tests'.

import { expect, test } from "bun:test";
import { render, screen, within } from "@testing-library/react";
import { BannerHead } from "./banner-head";

const banner = "https://media.example/dusk.png";
const lead = <span>AC</span>;
const end = <button type="button">Follow</button>;

/** The addresses of the pictures in `part` that are decorations. */
const decorations = (part: HTMLElement) =>
  within(part)
    .queryAllByRole<HTMLImageElement>("presentation")
    .map((picture) => picture.src);

test("a banner head is a region named by the element it is told of, with its banner as a decoration, its lead, its action and the rest inside", () => {
  render(
    <BannerHead banner={banner} lead={lead} end={end} labelledBy="head-name" dir="ltr">
      <h2 id="head-name">Alice Chen</h2>
      <p>Bakes on weekends.</p>
    </BannerHead>,
  );

  const head = screen.getByRole("region", { name: "Alice Chen" });
  expect(decorations(head)).toEqual([banner]);
  expect(within(head).queryAllByRole("img")).toHaveLength(0);
  expect(within(head).getByText("AC")).not.toBeNull();
  expect(within(head).getAllByRole("button", { name: "Follow" })).toHaveLength(1);
  expect(within(head).getByRole("heading", { level: 2, name: "Alice Chen" })).not.toBeNull();
  expect(within(head).getByText("Bakes on weekends.")).not.toBeNull();
});

test("a banner head with no banner shows no picture in its place, and with no action none", () => {
  render(
    <BannerHead lead={lead} labelledBy="head-name" dir="ltr">
      <h2 id="head-name">Alice Chen</h2>
    </BannerHead>,
  );

  const head = screen.getByRole("region", { name: "Alice Chen" });
  expect(decorations(head)).toEqual([]);
  expect(within(head).queryAllByRole("button")).toHaveLength(0);
  expect(within(head).getByText("AC")).not.toBeNull();
});

test("as a strip inside a section of another screen it is a group of that name, and no landmark of its own", () => {
  const { container } = render(
    <section aria-labelledby="section-title">
      <h2 id="section-title">Your profile</h2>
      <BannerHead strip banner={banner} lead={lead} end={end} labelledBy="strip-name" dir="ltr">
        <p id="strip-name">Alice Chen</p>
      </BannerHead>
    </section>,
  );

  // The one region is the section it stands in.
  expect(
    within(container)
      .getAllByRole("region")
      .map((region) => region.getAttribute("aria-labelledby")),
  ).toEqual(["section-title"]);
  const head = within(container).getByRole("group", { name: "Alice Chen" });
  expect(decorations(head)).toEqual([banner]);
  expect(within(head).getAllByRole("button", { name: "Follow" })).toHaveLength(1);
  expect(within(head).getByText("AC")).not.toBeNull();
});

test("a strip with no banner shows no picture in its place", () => {
  const { container } = render(
    <BannerHead strip lead={lead} labelledBy="strip-name" dir="ltr">
      <p id="strip-name">Alice Chen</p>
    </BannerHead>,
  );

  expect(decorations(within(container).getByRole("group", { name: "Alice Chen" }))).toEqual([]);
});

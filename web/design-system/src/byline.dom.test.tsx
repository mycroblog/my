// SPDX-License-Identifier: AGPL-3.0-only

// Where a byline's avatar leads: to the profile its name leads to, for the pointer alone, so that assistive technology
// and the keyboard meet one link to the profile. The rest of the byline is in entry.dom.test.tsx.

import { expect, test } from "bun:test";
import { NavigationProvider } from "@my/design-system";
import { isInaccessible, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { Avatar } from "./avatar";
import { Byline } from "./entry";

/** The avatar's image in the document, which assistive technology does not meet. */
function avatarImage() {
  const image = screen
    .getAllByRole("presentation", { hidden: true })
    .find((element) => element.getAttribute("src") === "/sara.png");
  if (image === undefined) throw new Error("test: the byline drew no avatar");
  return image;
}

test("a byline's avatar leads to the profile in place, as its name does, and is not a second link to hear or tab to", async () => {
  const user = userEvent.setup();
  const went: string[] = [];
  render(
    <NavigationProvider navigate={(href) => went.push(href)}>
      <Byline
        avatar={<Avatar name="Sara" src="/sara.png" size="post" decorative />}
        name="Sara"
        handle="@sara@example.social"
        href="/@sara"
      />
    </NavigationProvider>,
  );

  const avatar = avatarImage().closest("a");
  expect(avatar?.getAttribute("href")).toBe("/@sara");
  expect(avatar?.getAttribute("tabindex")).toBe("-1");
  expect(avatar?.getAttribute("aria-hidden")).toBe("true");
  expect(avatar !== null && isInaccessible(avatar)).toBe(true);
  // The name is the one link to the profile assistive technology meets.
  const links = screen.getAllByRole("link");
  expect(links.map((link) => [link.textContent, link.getAttribute("href")])).toEqual([["Sara", "/@sara"]]);

  await user.tab();
  expect(document.activeElement === links[0]).toBe(true);
  await user.tab();
  expect(document.activeElement === document.body).toBe(true);

  if (avatar !== null) await user.click(avatar);
  expect(went).toEqual(["/@sara"]);
});

test("a byline without a profile to lead to has an avatar that leads nowhere", () => {
  render(<Byline avatar={<Avatar name="Sara" src="/sara.png" size="post" decorative />} name="Sara" handle="@sara" />);

  expect(avatarImage().closest("a")).toBeNull();
  expect(screen.queryAllByRole("link", { hidden: true })).toHaveLength(0);
});

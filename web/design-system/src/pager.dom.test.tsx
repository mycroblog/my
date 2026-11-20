// SPDX-License-Identifier: AGPL-3.0-only

// The pager (#3279), as assistive technology and the keyboard get it: Previous and Next a navigation that keeps both
// in place, disabled where there is no page, which page is shown in a status, Try again on the button that failed;
// Show more gone once the list is read to its end, the same button named Try again after it failed, and the focus on
// the first item a press added. How each stands at a width is its stories' (Pager).

import { expect, mock, test } from "bun:test";
import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { Pager, ShowMore, useAddedFocus } from "./pager";

test("the pages are a navigation that keeps Previous and Next in place, disabled where there is no page", () => {
  const { rerender } = render(<Pager next={{ href: "/moderation/reports?after=1035" }} position={1} />);
  const pages = screen.getByRole("navigation", { name: "Pages" });
  expect(
    within(pages)
      .getAllByRole("link")
      .map((l) => l.textContent),
  ).toEqual(["Next"]);
  // The first page keeps its Previous where it stands, disabled, so that the focus never falls out of the pages.
  expect(within(pages).getByRole<HTMLButtonElement>("button", { name: "Previous" }).disabled).toBe(true);
  expect(within(pages).getByRole("status").textContent).toBe("Page 1");

  rerender(<Pager previous={{ onPress: () => undefined }} next={{ onPress: () => undefined }} />);
  expect(
    within(screen.getByRole("navigation", { name: "Pages" }))
      .getAllByRole("button")
      .map((b) => b.textContent),
  ).toEqual(["Previous", "Next"]);

  rerender(<Pager />);
  expect(screen.queryByRole("navigation")).toBeNull();
});

test("a page that failed says so, and its own button, named Try again, asks for it again", async () => {
  const user = userEvent.setup();
  const next = mock(() => undefined);
  render(<Pager previous={{ onPress: () => undefined }} next={{ onPress: next }} failed="next" position={3} />);
  // It names the page that failed, not the one shown.
  expect(screen.getByRole("alert").textContent).toBe("The next page could not be loaded.");
  const pages = screen.getByRole("navigation", { name: "Pages" });
  expect(
    within(pages)
      .getAllByRole("button")
      .map((b) => b.textContent),
  ).toEqual(["Previous", "Try again"]);
  await user.click(within(pages).getByRole("button", { name: "Try again" }));
  expect(next).toHaveBeenCalledTimes(1);
});

test("a window of periods names its two ways Later and Earlier and says what it shows", () => {
  render(<Pager periods="month" next={{ onPress: () => undefined }} position="Jun – Nov 2026" />);
  const pages = screen.getByRole("navigation", { name: "Pages" });
  expect(within(pages).getByRole<HTMLButtonElement>("button", { name: "Later months" }).disabled).toBe(true);
  expect(within(pages).getByRole("button", { name: "Earlier months" })).toBeTruthy();
  expect(within(pages).getByRole("status").textContent).toBe("Jun – Nov 2026");
});

test("a window of periods that failed names the periods, not a page", () => {
  render(
    <Pager periods="month" previous={{ onPress: () => undefined }} next={{ onPress: () => undefined }} failed="next" />,
  );
  expect(screen.getByRole("alert").textContent).toBe("The earlier months could not be loaded.");
  expect(screen.getByRole("button", { name: "Try again" })).toBeTruthy();
});

test("Show more stays the one button, named Try again after it failed, and goes with the list's end", () => {
  const { rerender } = render(<ShowMore more onPress={() => undefined} />);
  const button = screen.getByRole("button", { name: "Show more" });
  button.focus();
  rerender(<ShowMore more failed onPress={() => undefined} />);
  expect(screen.getByRole("alert").textContent).toBe("The rest of the list could not be loaded.");
  // The same element, renamed: the focus it held stays on it.
  expect(screen.getByRole("button", { name: "Try again" })).toBe(button);
  expect(document.activeElement).toBe(button);
  rerender(<ShowMore more={false} onPress={() => undefined} />);
  expect(screen.queryByRole("button")).toBeNull();
});

/** A list of `total` items that shows two more a press, once it is answered. */
function Growing({ total }: { total: number }) {
  const [shown, setShown] = useState(2);
  const [asked, setAsked] = useState(false);
  const added = useAddedFocus(shown);
  return (
    <>
      <ul>
        {Array.from({ length: shown }, (_, i) => (
          <li key={i} ref={added.item(i)}>
            {`Item ${String(i + 1)}`}
          </li>
        ))}
      </ul>
      <ShowMore
        more={shown < total}
        busy={asked}
        onPress={added.press(() => {
          setAsked(true);
        })}
      />
      <button
        type="button"
        onClick={() => {
          setAsked(false);
          setShown((n) => Math.min(n + 2, total));
        }}
      >
        Answer
      </button>
    </>
  );
}

test("the first item a press of Show more added takes the focus, the last press's too as Show more goes", async () => {
  const user = userEvent.setup();
  const { rerender } = render(<Growing total={4} />);
  await user.click(screen.getByRole("button", { name: "Show more" }));
  // Nothing added yet: the focus stays on the busy button.
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Show more" }));
  act(() => {
    screen.getByRole("button", { name: "Answer" }).click();
  });
  expect(document.activeElement?.textContent).toBe("Item 3");
  // The row took no focus of its own: it was given it, and keeps it through the render after, as its ref goes.
  expect(document.activeElement?.getAttribute("tabindex")).toBe("-1");
  // The list is read to its end: Show more is gone, and the focus was never on it.
  expect(screen.queryByRole("button", { name: "Show more" })).toBeNull();
  rerender(<Growing total={4} />);
  expect(document.activeElement?.textContent).toBe("Item 3");
  // A later render takes the focus nowhere.
  act(() => {
    screen.getByRole("button", { name: "Answer" }).focus();
    screen.getByRole("button", { name: "Answer" }).click();
  });
  expect(document.activeElement?.textContent).toBe("Answer");
});

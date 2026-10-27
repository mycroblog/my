// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { Menu, MenuItem } from "./Menu";
import { StageAvatar, StageGrid } from "./StageAvatar";

const labels = { speaking: "Talking", muted: "Microphone off", handRaised: "Hand raised" };

describe("StageAvatar", () => {
  it("is an item of the named list with the name and the caption, and a decorative avatar", () => {
    render(
      <StageGrid aria-label="Speakers">
        <StageAvatar name="Haruka" caption="Host" labels={labels} />
      </StageGrid>,
    );
    const item = within(screen.getByRole("list", { name: "Speakers" })).getByRole("listitem");
    expect(item).toHaveTextContent("Haruka");
    expect(item).toHaveTextContent("Host");
    expect(within(item).queryByRole("img")).toBeNull();
    expect(within(item).getByText("Haruka")).toHaveAttribute("dir", "auto");
  });

  it("says each state in words as well as drawing it", () => {
    render(
      <StageGrid aria-label="Speakers">
        <StageAvatar name="Haruka" speaking muted handRaised reaction={{ content: "❤️", label: "Reacted with ❤️" }} labels={labels} />
      </StageGrid>,
    );
    const item = screen.getByRole("listitem");
    for (const text of ["Talking", "Microphone off", "Hand raised", "Reacted with ❤️"]) expect(within(item).getByText(text)).toBeInTheDocument();
    // The drawn reaction is hidden; its label is read instead.
    expect(within(item).getByText("❤️")).toHaveAttribute("aria-hidden", "true");
  });

  it("shows no state it is not given", () => {
    render(
      <StageGrid aria-label="Listeners" size="listener">
        <StageAvatar name="Yui" size="listener" labels={labels} />
      </StageGrid>,
    );
    const item = screen.getByRole("listitem");
    for (const text of ["Talking", "Microphone off", "Hand raised"]) expect(within(item).queryByText(text)).toBeNull();
  });

  it("opens the menu of operations from the avatar with the keyboard", async () => {
    const user = userEvent.setup();
    const onAction = vi.fn();
    render(
      <StageGrid aria-label="Speakers">
        <StageAvatar
          name="Mio"
          caption="Speaker"
          labels={labels}
          menu={
            <Menu aria-label="Mio" onAction={onAction}>
              <MenuItem id="listener">Move to listeners</MenuItem>
            </Menu>
          }
        />
      </StageGrid>,
    );
    const button = screen.getByRole("button", { name: /Mio/ });
    expect(button).toHaveAttribute("aria-haspopup", "true");
    await user.tab();
    expect(button).toHaveFocus();
    await user.keyboard("{Enter}");
    await user.click(await screen.findByRole("menuitem", { name: "Move to listeners" }));
    expect(onAction.mock.calls[0]?.[0]).toBe("listener");
  });
});

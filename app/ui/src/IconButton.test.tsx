// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { IconButton, IconButtonLink, IconToggleButton } from "./IconButton";
import { RepliesIcon, StarIcon } from "./Icons";

describe("IconButton", () => {
  it("is named by its label and shows the label as its tooltip on keyboard focus (G12)", async () => {
    const user = userEvent.setup();
    const onPress = vi.fn();
    render(
      <IconButton label="Reply" onPress={onPress}>
        <StarIcon />
      </IconButton>,
    );
    const button = screen.getByRole("button", { name: "Reply" });
    expect(screen.queryByRole("tooltip")).not.toBeInTheDocument();

    await user.tab();
    expect(button).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Reply");

    await user.keyboard("{Enter}");
    expect(onPress).toHaveBeenCalledOnce();
  });

  it("keeps the description it is given (a count) while the tooltip, which repeats its name, is open", async () => {
    const user = userEvent.setup();
    render(
      <>
        <IconButton label="Reply" aria-describedby="count">
          <StarIcon />
        </IconButton>
        <span id="count">3</span>
      </>,
    );
    const button = screen.getByRole("button", { name: "Reply" });
    expect(button).toHaveAttribute("aria-describedby", "count");
    await user.tab();
    await screen.findByRole("tooltip");
    expect(button).toHaveAttribute("aria-describedby", "count");
  });
});

describe("IconToggleButton", () => {
  it("keeps its name in both states, exposes the state with aria-pressed, and shows the name as its tooltip", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(
      <IconToggleButton label="Favourite" isSelected={false} onChange={onChange}>
        <StarIcon />
      </IconToggleButton>,
    );
    const button = screen.getByRole("button", { name: "Favourite", pressed: false });
    await user.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Favourite");
    await user.keyboard(" ");
    expect(onChange).toHaveBeenCalledWith(true);

    rerender(
      <IconToggleButton label="Favourite" isSelected onChange={onChange}>
        <StarIcon filled />
      </IconToggleButton>,
    );
    expect(screen.getByRole("button", { name: "Favourite", pressed: true })).toBe(button);
  });
});

describe("IconButtonLink", () => {
  it("is a link named by its label, which its tooltip shows on keyboard focus (G12)", async () => {
    const user = userEvent.setup();
    render(
      <IconButtonLink label="Read 3 replies" href="/@alice/status/1">
        <RepliesIcon />3
      </IconButtonLink>,
    );
    const link = screen.getByRole("link", { name: "Read 3 replies" });
    expect(link).toHaveAttribute("href", "/@alice/status/1");

    await user.tab();
    expect(link).toHaveFocus();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Read 3 replies");
  });
});

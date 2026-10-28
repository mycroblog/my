// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { ToggleButton } from "./ToggleButton";

describe("ToggleButton", () => {
  it("is a button that exposes its state with aria-pressed and keeps its name", () => {
    const { rerender } = render(<ToggleButton aria-label="Favourite" isSelected={false} />);
    const button = screen.getByRole("button", { name: "Favourite" });
    expect(button).toHaveAttribute("aria-pressed", "false");
    rerender(<ToggleButton aria-label="Favourite" isSelected />);
    expect(screen.getByRole("button", { name: "Favourite", pressed: true })).toBe(button);
  });

  it("asks for the other state when pressed with a pointer, Enter or Space", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ToggleButton aria-label="Favourite" isSelected={false} onChange={onChange} />);
    const button = screen.getByRole("button", { name: "Favourite" });

    await user.click(button);
    await user.keyboard("{Enter}");
    await user.keyboard(" ");
    expect(onChange.mock.calls).toEqual([[true], [true], [true]]);
    // Controlled: the state is the caller's until it passes another.
    expect(button).toHaveAttribute("aria-pressed", "false");
  });

  it("ignores presses while disabled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<ToggleButton aria-label="Boost" isDisabled onChange={onChange} />);
    await user.click(screen.getByRole("button", { name: "Boost" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("colors the selected state by its tone", () => {
    render(<ToggleButton aria-label="Boost" tone="success" isSelected />);
    expect(screen.getByRole("button", { name: "Boost" })).toHaveClass("data-selected:text-success");
  });

  // A press shows at once where it happened (web-client/21-experience-principles.md P2, ADR-0110 §6): the button is
  // pressed (React Aria's data-pressed) from the pointer's down, it dents then, and it takes the sunken fill, which is
  // the answer with reduced motion.
  it("changes its look as soon as the pointer presses it, before the press ends", async () => {
    const user = userEvent.setup();
    render(<ToggleButton aria-label="Favourite" isSelected={false} />);
    const button = screen.getByRole("button", { name: "Favourite" });
    expect(button).not.toHaveAttribute("data-pressed");

    await user.pointer({ keys: "[MouseLeft>]", target: button });
    expect(button).toHaveAttribute("data-pressed", "true");
    expect(button).toHaveClass("motion-safe:data-pressed:scale-95", "data-pressed:bg-surface-sunken");

    await user.pointer({ keys: "[/MouseLeft]", target: button });
    expect(button).not.toHaveAttribute("data-pressed");
  });
});

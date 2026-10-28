// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { CallDock, CallDockButton } from "./CallDock";
import { EmojiIcon, LeaveIcon, MicrophoneIcon, RaiseHandIcon } from "./Icons";

describe("CallDock", () => {
  it("is a named toolbar whose arrow keys move between its buttons", async () => {
    const user = userEvent.setup();
    render(
      <CallDock
        label="Call controls"
        end={<CallDockButton label="Leave" icon={<LeaveIcon />} showLabel variant="danger" />}
      >
        <CallDockButton label="Reactions" icon={<EmojiIcon />} />
        <CallDockButton label="Hands" icon={<RaiseHandIcon />} />
      </CallDock>,
    );
    expect(screen.getByRole("toolbar", { name: "Call controls" })).toBeInTheDocument();
    await user.tab();
    expect(screen.getByRole("button", { name: "Reactions" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Hands" })).toHaveFocus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Leave" })).toHaveFocus();
    await user.keyboard("{ArrowLeft}");
    expect(screen.getByRole("button", { name: "Hands" })).toHaveFocus();
  });

  it("writes its height for the app's scroll padding while it shows", () => {
    const { unmount } = render(
      <CallDock label="Call controls">
        <CallDockButton label="Reactions" icon={<EmojiIcon />} />
      </CallDock>,
    );
    expect(document.documentElement.style.getPropertyValue("--call-bar-block-size")).toBe("0px");
    unmount();
    expect(document.documentElement.style.getPropertyValue("--call-bar-block-size")).toBe("");
  });
});

describe("CallDockButton", () => {
  it("is a toggle with aria-pressed and the same name in both states", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(
      <CallDockButton label="Microphone" icon={<MicrophoneIcon />} showLabel isSelected={false} onChange={onChange} />,
    );
    const button = screen.getByRole("button", { name: "Microphone", pressed: false });
    await user.click(button);
    expect(onChange).toHaveBeenCalledWith(true);
    rerender(
      <CallDockButton label="Microphone" icon={<MicrophoneIcon filled />} showLabel isSelected onChange={onChange} />,
    );
    expect(screen.getByRole("button", { name: "Microphone", pressed: true })).toBe(button);
  });

  it("is a plain button without isSelected", async () => {
    const user = userEvent.setup();
    const onPress = vi.fn();
    render(<CallDockButton label="Leave" icon={<LeaveIcon />} variant="danger" onPress={onPress} />);
    const button = screen.getByRole("button", { name: "Leave" });
    expect(button).not.toHaveAttribute("aria-pressed");
    await user.click(button);
    expect(onPress).toHaveBeenCalledOnce();
  });

  it("is described by its count, and keeps its name", () => {
    render(
      <CallDockButton label="Hands" icon={<RaiseHandIcon />} showLabel count={{ value: 2, label: "2 raised hands" }} />,
    );
    expect(screen.getByRole("button", { name: "Hands" })).toHaveAccessibleDescription("2 raised hands");
  });

  it("shows its label as text or, for an icon only, as a tooltip on focus", async () => {
    const user = userEvent.setup();
    const { unmount } = render(<CallDockButton label="Hands" icon={<RaiseHandIcon />} showLabel />);
    expect(screen.getByRole("button", { name: "Hands" })).toHaveTextContent("Hands");
    unmount();
    render(<CallDockButton label="Reactions" icon={<EmojiIcon />} />);
    expect(screen.getByRole("button", { name: "Reactions" })).toHaveTextContent("");
    await user.tab();
    expect(await screen.findByRole("tooltip")).toHaveTextContent("Reactions");
  });
});

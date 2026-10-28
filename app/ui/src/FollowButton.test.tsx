// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { FollowButton } from "./FollowButton";

describe("FollowButton", () => {
  it("is the filled main action while not followed, and quiet once followed", () => {
    const { rerender } = render(<FollowButton status="none" label="Follow" />);
    expect(screen.getByRole("button", { name: "Follow" })).toHaveClass("bg-accent-fill");
    rerender(<FollowButton status="following" label="Following" />);
    const following = screen.getByRole("button", { name: "Following" });
    expect(following).not.toHaveClass("bg-accent-fill");
    expect(following).toHaveClass("border-accent");
  });

  it("keeps its place while the relationship loads, without a control", () => {
    const { container } = render(<FollowButton status="loading" label="" />);
    expect(screen.queryByRole("button")).toBeNull();
    // eslint-disable-next-line testing-library/no-container, testing-library/no-node-access -- the placeholder is decorative and has no role.
    const placeholder = container.querySelector("[data-follow-placeholder]");
    expect(placeholder).toHaveAttribute("aria-hidden", "true");
    expect(placeholder).toHaveClass("h-10");
  });
});

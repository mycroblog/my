// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { FilteredPost } from "./FilteredPost";

describe("FilteredPost", () => {
  it("is an article named only by what the post matched, with the button that shows it", async () => {
    const user = userEvent.setup();
    const onShow = vi.fn();
    render(<FilteredPost label="Filtered: Spoilers" showLabel="Show" onShow={onShow} />);

    const article = screen.getByRole("article", { name: "Filtered: Spoilers" });
    expect(article).not.toHaveAttribute("aria-describedby");
    await user.click(screen.getByRole("button", { name: "Show" }));
    expect(onShow).toHaveBeenCalledOnce();
  });
});

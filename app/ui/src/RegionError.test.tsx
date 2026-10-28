// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RegionError } from "./RegionError";

describe("RegionError", () => {
  it("shows what happened, the reason and a retry button, without a live region by default", async () => {
    const onRetry = vi.fn();
    render(
      <RegionError heading="Couldn't load the posts" reason="Timed out." retryLabel="Try again" onRetry={onRetry} />,
    );
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.queryByRole("heading")).not.toBeInTheDocument();
    expect(screen.getByText("Couldn't load the posts")).toBeInTheDocument();
    expect(screen.getByText("Timed out.")).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole("button", { name: "Try again" }));
    expect(onRetry).toHaveBeenCalledOnce();
  });

  it("is an alert only when asked", () => {
    render(<RegionError heading="Couldn't load the posts" alert />);
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't load the posts");
  });

  it("takes a heading level", () => {
    render(<RegionError heading="Couldn't load the replies" headingLevel={3} />);
    expect(screen.getByRole("heading", { level: 3, name: "Couldn't load the replies" })).toBeInTheDocument();
  });

  it("has no button without onRetry, and a waiting one while retrying", () => {
    const { rerender } = render(<RegionError heading="Not available" />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    rerender(<RegionError heading="Failed" retryLabel="Try again" onRetry={() => undefined} isRetrying />);
    expect(screen.getByRole("button", { name: "Try again" })).toHaveAttribute("data-pending", "true");
  });
});

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

  describe("through a retry", () => {
    /** The error of a region, with `isRetrying` as given. */
    function region(isRetrying: boolean) {
      return (
        <RegionError
          heading="Failed"
          reason="Why"
          retryLabel="Try again"
          onRetry={() => undefined}
          isRetrying={isRetrying}
        />
      );
    }

    it("is an alert only when a retry from its button ends with the error still here, each time", async () => {
      const user = userEvent.setup();
      const { rerender } = render(region(false));
      const button = screen.getByRole("button", { name: "Try again" });

      await user.click(button);
      // Pressed, and the caller has not said yet that it retries: this is not the result.
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      rerender(region(true));
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      expect(button).toHaveFocus();

      rerender(region(false));
      const alert = screen.getByRole("alert");
      expect(alert).toHaveTextContent("FailedWhy");
      expect(alert).not.toContainElement(button);
      expect(button).toHaveFocus();

      await user.click(button);
      rerender(region(true));
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      rerender(region(false));
      // A new element, since a live region announces only what is added to it.
      expect(screen.getByRole("alert")).not.toBe(alert);
    });

    it("is no alert when a retry it did not start ends", () => {
      const { rerender } = render(region(true));
      rerender(region(false));
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    });

    it("leaves `alert` off during a retry", () => {
      const props = { heading: "Failed", retryLabel: "Try again", onRetry: () => undefined, alert: true };
      const { rerender } = render(<RegionError {...props} />);
      const alert = screen.getByRole("alert");
      rerender(<RegionError {...props} isRetrying />);
      expect(screen.queryByRole("alert")).not.toBeInTheDocument();
      rerender(<RegionError {...props} />);
      expect(screen.getByRole("alert")).not.toBe(alert);
    });
  });

  describe("when a retry succeeds and the error goes", () => {
    function Page({ failed, dialog = false }: { failed: boolean; dialog?: boolean }) {
      const content = failed ? (
        <RegionError heading="Failed" retryLabel="Try again" onRetry={() => undefined} />
      ) : (
        <p>Loaded</p>
      );
      return (
        <main>
          <h1 tabIndex={-1}>Page</h1>
          <h2>Not focusable</h2>
          {dialog ? (
            <div role="dialog" aria-label="Dialog" tabIndex={-1}>
              {content}
            </div>
          ) : (
            content
          )}
          <h2 tabIndex={-1}>Below</h2>
        </main>
      );
    }

    it("moves focus from its button to the nearest focusable heading above it", async () => {
      const { rerender } = render(<Page failed />);
      await userEvent.setup().click(screen.getByRole("button", { name: "Try again" }));
      rerender(<Page failed={false} />);
      expect(screen.getByRole("heading", { level: 1, name: "Page" })).toHaveFocus();
    });

    it("stays in its dialog", async () => {
      const { rerender } = render(<Page failed dialog />);
      await userEvent.setup().click(screen.getByRole("button", { name: "Try again" }));
      rerender(<Page failed={false} dialog />);
      expect(screen.getByRole("dialog", { name: "Dialog" })).toHaveFocus();
    });

    it("leaves focus alone when focus has moved away from its button", async () => {
      const user = userEvent.setup();
      const { rerender } = render(
        <>
          <Page failed />
          <button type="button">Elsewhere</button>
        </>,
      );
      await user.click(screen.getByRole("button", { name: "Try again" }));
      await user.click(screen.getByRole("button", { name: "Elsewhere" }));
      rerender(
        <>
          <Page failed={false} />
          <button type="button">Elsewhere</button>
        </>,
      );
      expect(screen.getByRole("button", { name: "Elsewhere" })).toHaveFocus();
    });

    it("leaves focus alone without a retry", () => {
      const { rerender } = render(<Page failed />);
      screen.getByRole("button", { name: "Try again" }).focus();
      rerender(<Page failed={false} />);
      expect(screen.getByRole("heading", { level: 1, name: "Page" })).not.toHaveFocus();
    });
  });
});

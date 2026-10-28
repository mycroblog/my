// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { ExternalLink } from "./ExternalLink";
import { OfflineBanner } from "./OfflineBanner";

describe("OfflineBanner", () => {
  it("keeps one status region and fills it only while offline", () => {
    const { rerender } = render(<OfflineBanner isOffline={false}>You're offline.</OfflineBanner>);
    const status = screen.getByRole("status");
    expect(status).toBeEmptyDOMElement();
    rerender(<OfflineBanner isOffline>You're offline.</OfflineBanner>);
    expect(screen.getByRole("status")).toBe(status);
    expect(status).toHaveTextContent("You're offline.");
    rerender(<OfflineBanner isOffline={false}>You're offline.</OfflineBanner>);
    expect(status).toBeEmptyDOMElement();
  });

  it("puts the action after the message, outside the live region, only while offline", () => {
    const action = <ExternalLink href="https://status.example/">Status</ExternalLink>;
    const { rerender } = render(
      <OfflineBanner isOffline={false} action={action}>
        You're offline.
      </OfflineBanner>,
    );
    expect(screen.queryByRole("link")).not.toBeInTheDocument();
    rerender(
      <OfflineBanner isOffline action={action}>
        You're offline.
      </OfflineBanner>,
    );
    const status = screen.getByRole("status");
    const link = screen.getByRole("link", { name: "Status" });
    expect(status).toHaveTextContent("You're offline.");
    expect(status).not.toContainElement(link);
    // After the message in the document's order.
    expect(status.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("writes its height to --offline-bar-block-size while it is mounted, for the shell's heights", () => {
    const { unmount } = render(<OfflineBanner isOffline>You're offline.</OfflineBanner>);
    // jsdom lays nothing out, so the height is 0; the story tests check the layout in a browser.
    expect(document.documentElement.style.getPropertyValue("--offline-bar-block-size")).toBe("0px");
    unmount();
    expect(document.documentElement.style.getPropertyValue("--offline-bar-block-size")).toBe("");
  });
});

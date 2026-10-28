// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Link } from "./Link";
import { NoticeBanner } from "./NoticeBanner";

describe("NoticeBanner", () => {
  it("is a named region with the headline, the details and the actions in that order", () => {
    render(
      <NoticeBanner
        label="Closure"
        headline="This server closes on 1 Nov."
        actions={<Link href="/settings/account">Export</Link>}
      >
        <p>Thank you.</p>
      </NoticeBanner>,
    );
    const region = screen.getByRole("region", { name: "Closure" });
    expect(region).toHaveTextContent("This server closes on 1 Nov.");
    const details = screen.getByText("Thank you.");
    const link = screen.getByRole("link", { name: "Export" });
    expect(region).toContainElement(details);
    expect(details.compareDocumentPosition(link) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("has no live region, so that it does not interrupt", () => {
    render(<NoticeBanner label="Closure" headline="Closing." />);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });
});

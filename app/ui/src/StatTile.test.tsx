// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatTile, StatTileGrid } from "./StatTile";

describe("StatTile", () => {
  it("says its name, value, change and state in text", () => {
    const { container } = render(
      <StatTile
        label="Failed jobs"
        value="4"
        status="danger"
        statusLabel="Failing"
        change={{ direction: "up", text: "+4", period: "in the last hour", tone: "negative" }}
      />,
    );
    expect(container).toHaveTextContent("Failed jobsFailing4+4in the last hour");
  });
});

describe("StatTile with href", () => {
  it("is one link named by its label, inside a list of tiles", () => {
    render(
      <StatTileGrid aria-label="Summary">
        <StatTile as="li" label="Open reports" value="3" href="/moderation/reports" />
        <StatTile as="li" label="Failed jobs" value="0" />
      </StatTileGrid>,
    );
    expect(screen.getByRole("list", { name: "Summary" })).toBeInTheDocument();
    expect(screen.getAllByRole("listitem")).toHaveLength(2);
    expect(screen.getByRole("link", { name: "Open reports" })).toHaveAttribute("href", "/moderation/reports");
    expect(screen.getAllByRole("link")).toHaveLength(1);
  });
});

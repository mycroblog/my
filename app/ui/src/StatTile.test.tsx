// SPDX-License-Identifier: AGPL-3.0-only
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StatTile } from "./StatTile";

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

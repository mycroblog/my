// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Section } from "./Section";
import { TriageLayout } from "./TriageLayout";

describe("TriageLayout", () => {
  it("puts the decision after the summary and before the evidence in the document", () => {
    render(
      <TriageLayout
        summary={<Section title="Summary">The report</Section>}
        decision={<Section title="Resolution">Resolve</Section>}
        evidence={
          <>
            <Section title="Posts">A post</Section>
            <Section title="Notes">A note</Section>
          </>
        }
      />,
    );
    expect(screen.getAllByRole("heading", { level: 2 }).map((heading) => heading.textContent)).toEqual([
      "Summary",
      "Resolution",
      "Posts",
      "Notes",
    ]);
  });
});

// SPDX-License-Identifier: AGPL-3.0-only
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import * as Icons from "./Icons";

const names = Object.keys(Icons.iconTable) as (keyof typeof Icons.iconTable)[];

// The Storybook gallery (Foundations/Icons) checks that every icon is decorative, sized by its place and
// mirrored only when it has a direction.
describe("iconTable", () => {
  it("names the Phosphor icons it draws with, for the Storybook gallery", () => {
    for (const name of names) {
      const entry = Icons.iconTable[name];
      expect(entry.off.icon.name, name).toBe(entry.off.phosphor);
      if ("on" in entry) expect(entry.on.icon.name, name).toBe(entry.on.phosphor);
    }
  });
});

describe("VisibilityIcon", () => {
  it("draws nothing for a level this bundle does not know", () => {
    const { container } = render(<Icons.VisibilityIcon visibility={"not_yet_known" as "public"} />);
    expect(container).toBeEmptyDOMElement();
  });
});

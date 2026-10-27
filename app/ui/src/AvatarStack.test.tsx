// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { AvatarStack } from "./AvatarStack";

const avatars = [
  { id: "1", name: "Haruka", speaking: true },
  { id: "2", name: "Sota" },
  { id: "3", name: "Mio" },
  { id: "4", name: "Kenji" },
];

describe("AvatarStack", () => {
  it("shows the first three as decorative avatars", () => {
    render(<AvatarStack avatars={avatars} />);
    expect(screen.queryByRole("img")).toBeNull();
    for (const initial of ["H", "S", "M"]) expect(screen.getByText(initial)).toBeInTheDocument();
    expect(screen.queryByText("K")).toBeNull();
  });

  it("shows as many as `max`", () => {
    render(<AvatarStack avatars={avatars} max={4} />);
    expect(screen.getByText("K")).toBeInTheDocument();
  });

  it("renders nothing without avatars", () => {
    const { container } = render(<AvatarStack avatars={[]} />);
    expect(container).toBeEmptyDOMElement();
  });
});

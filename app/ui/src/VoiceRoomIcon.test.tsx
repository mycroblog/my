// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { VoiceRoomIcon } from "./VoiceRoomIcon";

const blob = { shortcode: "blob", url: "/emoji/blob.gif", static_url: "/emoji/blob.png" };

describe("VoiceRoomIcon", () => {
  it("shows a Unicode emoji, hidden from assistive technology", () => {
    render(
      <h1>
        <VoiceRoomIcon emoji={{ unicode: "\u{1F3B5}" }} />
        Friday night radio
      </h1>,
    );
    expect(screen.getByText("\u{1F3B5}")).toHaveAttribute("aria-hidden", "true");
    expect(screen.getByRole("heading", { name: "Friday night radio" })).toHaveTextContent(
      "\u{1F3B5}Friday night radio",
    );
  });

  it("draws a custom emoji as its image, which does not name the heading", () => {
    render(
      <h1>
        <VoiceRoomIcon emoji={{ custom: blob }} />
        Friday night radio
      </h1>,
    );
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
    expect(screen.getByRole("img", { hidden: true, name: ":blob:" })).toHaveAttribute("src", "/emoji/blob.png");
    expect(screen.getByRole("heading", { name: "Friday night radio" })).toBeInTheDocument();
  });
});

// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Link } from "./Link";
import { VoiceRoomCard, type VoiceRoomCardProps } from "./VoiceRoomCard";

const props: VoiceRoomCardProps = {
  name: "Friday night radio",
  href: "/voice/1",
  liveLabel: "Live",
  stats: "12 people · 38 min",
  speakers: [
    { id: "1", name: "Alice" },
    { id: "2", name: "Sota" },
  ],
  host: <Link href="/@alice">Alice</Link>,
  hostLabel: "Host",
  description: "Talking about music.",
  tags: ["#music"],
  tagsLabel: "Hashtags",
};

function renderCard(overrides: Partial<VoiceRoomCardProps> = {}) {
  return render(
    <ul>
      <VoiceRoomCard {...props} {...overrides} />
    </ul>,
  );
}

describe("VoiceRoomCard", () => {
  it("shows the Live pill, the stats, the name as a heading link, the host, the description and the hashtags", () => {
    renderCard();
    // The card is the first item; the hashtags are items of a list inside it.
    const [item] = screen.getAllByRole("listitem");
    if (item === undefined) throw new Error("no card");
    expect(within(item).getByText("Live")).toBeInTheDocument();
    expect(item).toHaveTextContent("12 people · 38 min");
    expect(within(item).getByRole("heading", { level: 2, name: "Friday night radio" })).toBeInTheDocument();
    expect(within(item).getByRole("link", { name: "Friday night radio" })).toHaveAttribute("href", "/voice/1");
    expect(within(item).getByRole("link", { name: "Alice" })).toHaveAttribute("href", "/@alice");
    expect(item).toHaveTextContent("Host");
    expect(item).toHaveTextContent("Talking about music.");
    expect(within(item).getByRole("list", { name: "Hashtags" })).toBeInTheDocument();
  });

  it("stacks the speakers as decorative avatars", () => {
    renderCard();
    expect(screen.queryByRole("img")).toBeNull();
    expect(screen.getByText("S")).toBeInTheDocument();
  });

  it("leaves out the description and the hashtags on the plain surface", () => {
    renderCard({ surface: "plain", headingLevel: 3 });
    expect(screen.getByRole("heading", { level: 3, name: "Friday night radio" })).toBeInTheDocument();
    expect(screen.queryByText("Talking about music.")).toBeNull();
    expect(screen.queryByRole("list", { name: "Hashtags" })).toBeNull();
  });
});

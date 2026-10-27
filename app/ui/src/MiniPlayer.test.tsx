// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { CallDockButton } from "./CallDock";
import { LeaveIcon } from "./Icons";
import { Link } from "./Link";
import { MiniPlayer } from "./MiniPlayer";

const avatars = [
  { id: "1", name: "Haruka", speaking: true },
  { id: "2", name: "Sota" },
  { id: "3", name: "Mio" },
  { id: "4", name: "Kenji" },
];

function renderPlayer(notice?: string) {
  return render(
    <MiniPlayer
      label="Voice chat call"
      avatars={avatars}
      title={
        <Link href="/voice/1" variant="standalone">
          <span>Friday night radio</span>
        </Link>
      }
      status="Haruka is talking"
      notice={notice}
    >
      <CallDockButton label="Leave" size="sm" variant="danger-soft" icon={<LeaveIcon />} />
    </MiniPlayer>,
  );
}

describe("MiniPlayer", () => {
  it("is a named region with the room's link, the status and the controls", () => {
    renderPlayer();
    const region = screen.getByRole("region", { name: "Voice chat call" });
    expect(within(region).getByRole("link", { name: "Friday night radio" })).toHaveAttribute("href", "/voice/1");
    expect(region).toHaveTextContent("Haruka is talking");
    expect(within(region).getByRole("button", { name: "Leave" })).toBeInTheDocument();
    // The shell's stylesheet finds the bar of the call by this attribute.
    expect(region).toHaveAttribute("data-call-bar");
  });

  it("stacks the first three speakers as decorative avatars", () => {
    renderPlayer();
    const region = screen.getByRole("region", { name: "Voice chat call" });
    expect(within(region).queryByRole("img")).toBeNull();
    for (const initial of ["H", "S", "M"]) expect(within(region).getByText(initial)).toBeInTheDocument();
    expect(within(region).queryByText("K")).toBeNull();
  });

  it("shows a notice on a row of its own", () => {
    renderPlayer("No microphone was found.");
    expect(screen.getByRole("region", { name: "Voice chat call" })).toHaveTextContent("No microphone was found.");
  });

  it("writes its height for the app's scroll padding while it shows", () => {
    const { unmount } = renderPlayer();
    expect(document.documentElement.style.getPropertyValue("--call-bar-block-size")).toBe("0px");
    unmount();
    expect(document.documentElement.style.getPropertyValue("--call-bar-block-size")).toBe("");
  });
});

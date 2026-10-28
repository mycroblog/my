// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { userEvent } from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it } from "vitest";
import { AutosaveControl, SaveIndicator, type SaveState } from "./SaveIndicator";
import { Switch } from "./Switch";

const labels = { saving: "Saving…", saved: "Saved", failed: "Not saved" };

describe("SaveIndicator", () => {
  it("shows nothing before the first change", () => {
    const { container } = render(<SaveIndicator state="idle" labels={labels} />);
    expect(container).toBeEmptyDOMElement();
  });

  it.each([
    ["saving", "Saving…"],
    ["saved", "Saved"],
    ["failed", "Not saved"],
  ] as const)("shows the words of %s, and is no live region", (state, words) => {
    render(<SaveIndicator state={state} labels={labels} />);
    expect(screen.getByText(words)).toBeVisible();
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  it("keeps the words for assistive technology in a table's cell", () => {
    render(<SaveIndicator state="saved" labels={labels} display="icon" />);
    expect(screen.getByText("Saved")).toHaveClass("sr-only");
  });
});

function Autosaved({ save }: { save: () => Promise<void> }) {
  const [selected, setSelected] = useState(false);
  const [state, setState] = useState<SaveState>("idle");
  return (
    <AutosaveControl state={state} labels={labels}>
      <Switch
        isSelected={selected}
        onChange={(next) => {
          setSelected(next);
          setState("saving");
          void save().then(
            () => {
              setState("saved");
            },
            () => {
              setState("failed");
            },
          );
        }}
      >
        Autoplay
      </Switch>
    </AutosaveControl>
  );
}

describe("AutosaveControl", () => {
  it("shows saving right after the change and saved once it is kept, beside the control", async () => {
    const user = userEvent.setup();
    let finish: () => void = () => undefined;
    render(
      <Autosaved
        save={() =>
          new Promise<void>((resolve) => {
            finish = resolve;
          })
        }
      />,
    );
    const control = screen.getByRole("switch", { name: "Autoplay" });
    await user.click(control);
    const saving = screen.getByText("Saving…");
    expect(saving).toBeVisible();
    // Right after the control in the reading order.
    expect(control.compareDocumentPosition(saving) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    finish();
    expect(await screen.findByText("Saved")).toBeVisible();
    expect(screen.queryByText("Saving…")).not.toBeInTheDocument();
  });

  it("shows that a change was not saved", async () => {
    const user = userEvent.setup();
    render(<Autosaved save={() => Promise.reject(new Error("offline"))} />);
    await user.click(screen.getByRole("switch", { name: "Autoplay" }));
    expect(await screen.findByText("Not saved")).toBeVisible();
  });
});

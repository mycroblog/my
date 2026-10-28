// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Button } from "./Button";
import { FormActionBar } from "./FormActionBar";

describe("FormActionBar", () => {
  it("has its empty live regions from the first render", () => {
    render(
      <FormActionBar>
        <Button type="submit">Save</Button>
      </FormActionBar>,
    );
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
    expect(screen.getByRole("alert")).toBeEmptyDOMElement();
  });

  it("shows a success beside the button, in the status region", () => {
    const { rerender } = render(
      <FormActionBar>
        <Button type="submit">Save</Button>
      </FormActionBar>,
    );
    rerender(
      <FormActionBar result={{ tone: "success", message: "Saved your profile." }}>
        <Button type="submit">Save</Button>
      </FormActionBar>,
    );
    const status = screen.getByRole("status");
    expect(status).toHaveTextContent("Saved your profile.");
    // Right after the button in the reading order.
    const button = screen.getByRole("button", { name: "Save" });
    expect(button.compareDocumentPosition(status) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
    expect(screen.getByRole("alert")).toBeEmptyDOMElement();
  });

  it("shows a failure in the alert region", () => {
    render(
      <FormActionBar result={{ tone: "danger", message: "Couldn't save: the name is too long." }}>
        <Button type="submit">Save</Button>
      </FormActionBar>,
    );
    expect(screen.getByRole("alert")).toHaveTextContent("Couldn't save: the name is too long.");
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });
});

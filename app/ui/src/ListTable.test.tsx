// SPDX-License-Identifier: AGPL-3.0-only
import { render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { Button } from "./Button";
import { ListTable, type ListTableColumns } from "./ListTable";

type Relay = { id: string; inbox: string; state: string; added: string };

const relays: Relay[] = [
  { id: "r1", inbox: "relay.example/inbox", state: "Accepted", added: "Oct 1, 2026" },
  { id: "r2", inbox: "relay.example.org/inbox", state: "Pending", added: "Oct 2, 2026" },
];

const columns: ListTableColumns<Relay> = [
  { id: "inbox", header: "Inbox", wrap: "anywhere", cell: (relay) => relay.inbox },
  { id: "state", header: "State", cell: (relay) => relay.state },
  { id: "added", header: "Added", cell: (relay) => relay.added },
  {
    id: "actions",
    kind: "actions",
    header: "Actions",
    cell: (relay) => (
      <Button size="sm" aria-label={`Remove ${relay.inbox}`}>
        {"Remove"}
      </Button>
    ),
  },
];

function renderRelays() {
  return render(
    <ListTable items={relays} columns={columns} getId={(relay) => relay.id} caption="Relays" headingLevel={2} />,
  );
}

/**
 * A ResizeObserver, which jsdom lacks; with it, the table measures its container, whose width is 0 in jsdom (no
 * layout), below the narrow width.
 */
function narrowContainer() {
  vi.stubGlobal(
    "ResizeObserver",
    class {
      observe() {}
      disconnect() {}
    },
  );
}

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("ListTable", () => {
  it("draws a table whose rows are named by the first column, without a ResizeObserver to measure with", () => {
    renderRelays();
    const table = screen.getByRole("table", { name: "Relays" });
    expect(
      within(table)
        .getAllByRole("columnheader")
        .map((header) => header.textContent),
    ).toEqual(["Inbox", "State", "Added", "Actions"]);
    const header = within(table).getByRole("rowheader", { name: "relay.example/inbox" });
    expect(header).toHaveAttribute("tabindex", "-1");
    expect(header).toHaveAttribute("data-row-header");
    // The row carries the record's id, for focus to find it after a change (data-row-id).
    expect(within(table).getAllByRole("row")[1]).toHaveAttribute("data-row-id", "r1");
    expect(within(table).getByRole("button", { name: "Remove relay.example/inbox" })).toBeInTheDocument();
  });

  it("draws a list of cards in a narrow container, every value under its column header", () => {
    narrowContainer();
    renderRelays();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    const list = screen.getByRole("list", { name: "Relays" });
    const cards = within(list).getAllByRole("listitem");
    expect(cards).toHaveLength(relays.length);
    for (const [index, card] of cards.entries()) {
      const relay = relays[index];
      if (relay === undefined) throw new Error("a card without its relay");
      expect(card).toHaveAttribute("data-row-id", relay.id);
      // The first column is the card's heading, at the level asked for, and focusable from script.
      const heading = within(card).getByRole("heading", { level: 2, name: relay.inbox });
      expect(heading).toHaveAttribute("tabindex", "-1");
      expect(heading).toHaveAttribute("data-row-header");
      // Every other value keeps its column header as its term; the actions have no term.
      const terms = within(card).getAllByRole("term");
      expect(terms.map((term) => term.textContent)).toEqual(["State", "Added"]);
      expect(
        within(card)
          .getAllByRole("definition")
          .map((value) => value.textContent),
      ).toEqual([relay.state, relay.added]);
      expect(within(card).getByRole("button", { name: `Remove ${relay.inbox}` })).toBeInTheDocument();
    }
  });
});
